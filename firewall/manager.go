package firewall

import (
	"net"
	"sync"

	"emperror.dev/errors"
	"github.com/apex/log"
	"gorm.io/gorm"

	"github.com/priyxstudio/propel/internal/database"
	"github.com/priyxstudio/propel/internal/models"
)

// Manager handles firewall rule management and iptables operations
type Manager struct {
	mu sync.RWMutex
}

// NewManager creates a new firewall manager instance
func NewManager() *Manager {
	return &Manager{}
}

// executeIptables is now platform-specific, see manager_linux.go

// validateProtocol validates that protocol is tcp or udp
func validateProtocol(protocol string) error {
	if protocol != "tcp" && protocol != "udp" {
		return errors.Errorf("invalid protocol: %s (must be 'tcp' or 'udp')", protocol)
	}
	return nil
}

// buildIptablesRuleArgs builds iptables command arguments to prevent command injection
// Validates all inputs before building the command
// buildIptablesRuleArgs, getChainLength, calculateRulePosition are now platform-specific

// ApplyRule is now platform-specific

// RemoveRule, removeRuleUnlocked, applyRuleUnlocked are now platform-specific

// SyncRules syncs all firewall rules from database to iptables
func (m *Manager) SyncRules(serverUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get all active rules for this server
	var rules []models.FirewallRule
	if err := database.Instance().Where("server_uuid = ? AND deleted_at IS NULL", serverUUID).
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return errors.Wrap(err, "failed to fetch firewall rules")
	}

	if len(rules) == 0 {
		log.WithField("server", serverUUID).Debug("no firewall rules to sync")
		return nil
	}

	// Apply rules in priority order (using unlocked version since we already have the lock)
	appliedCount := 0
	failedCount := 0
	for _, rule := range rules {
		if err := m.applyRuleUnlocked(&rule); err != nil {
			log.WithError(err).WithField("rule_id", rule.ID).Warn("failed to apply firewall rule during sync")
			failedCount++
			// Continue with other rules
		} else {
			appliedCount++
		}
	}

	log.WithFields(log.Fields{
		"server":  serverUUID,
		"total":   len(rules),
		"applied": appliedCount,
		"failed":  failedCount,
	}).Info("synced firewall rules")

	return nil
}

// GetRules returns all firewall rules for a server
func (m *Manager) GetRules(serverUUID string) ([]models.FirewallRule, error) {
	var rules []models.FirewallRule
	if err := database.Instance().Where("server_uuid = ? AND deleted_at IS NULL", serverUUID).
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, errors.Wrap(err, "failed to fetch firewall rules")
	}
	return rules, nil
}

// CreateRule creates a new firewall rule
func (m *Manager) CreateRule(rule *models.FirewallRule) error {
	// Validate rule type
	if rule.Type != models.FirewallRuleTypeAllow && rule.Type != models.FirewallRuleTypeBlock {
		return errors.Errorf("invalid rule type: %s (must be 'allow' or 'block')", rule.Type)
	}

	// Validate protocol
	if rule.Protocol == "" {
		rule.Protocol = "tcp"
	}
	if rule.Protocol != "tcp" && rule.Protocol != "udp" {
		return errors.Errorf("invalid protocol: %s (must be 'tcp' or 'udp')", rule.Protocol)
	}

	// Validate port range
	if rule.ServerPort < 1 || rule.ServerPort > 65535 {
		return errors.Errorf("invalid port: %d (must be between 1 and 65535)", rule.ServerPort)
	}

	// Set default priority if not set
	if rule.Priority == 0 {
		rule.Priority = 100
	}

	// Save to database
	if err := database.Instance().Create(rule).Error; err != nil {
		return errors.Wrap(err, "failed to create firewall rule")
	}

	// Apply to iptables
	if err := m.ApplyRule(rule); err != nil {
		// If iptables apply fails, hard delete from database (rollback)
		if delErr := database.Instance().Unscoped().Delete(rule).Error; delErr != nil {
			log.WithError(delErr).WithField("rule_id", rule.ID).Error("failed to rollback firewall rule creation")
		}
		return errors.Wrap(err, "failed to apply firewall rule to iptables")
	}

	return nil
}

// UpdateRule updates an existing firewall rule
func (m *Manager) UpdateRule(ruleID uint, updates *models.FirewallRule) error {
	// Get existing rule
	var existingRule models.FirewallRule
	if err := database.Instance().First(&existingRule, ruleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return errors.Errorf("firewall rule %d not found", ruleID)
		}
		return errors.Wrap(err, "failed to fetch firewall rule")
	}

	// Remove old rule from iptables
	if err := m.RemoveRule(&existingRule); err != nil {
		log.WithError(err).Warn("failed to remove old firewall rule during update")
	}

	// Update fields
	if updates.RemoteIP != "" {
		existingRule.RemoteIP = updates.RemoteIP
	}
	if updates.ServerPort != 0 {
		existingRule.ServerPort = updates.ServerPort
	}
	if updates.Priority != 0 {
		existingRule.Priority = updates.Priority
	}
	if updates.Type != "" {
		existingRule.Type = updates.Type
	}
	if updates.Protocol != "" {
		existingRule.Protocol = updates.Protocol
	}

	// Validate updated rule
	if existingRule.Type != models.FirewallRuleTypeAllow && existingRule.Type != models.FirewallRuleTypeBlock {
		return errors.Errorf("invalid rule type: %s", existingRule.Type)
	}

	// Save to database
	if err := database.Instance().Save(&existingRule).Error; err != nil {
		return errors.Wrap(err, "failed to update firewall rule")
	}

	// Apply new rule to iptables
	if err := m.ApplyRule(&existingRule); err != nil {
		return errors.Wrap(err, "failed to apply updated firewall rule to iptables")
	}

	return nil
}

// DeleteRule deletes a firewall rule
// For block rules, this will unblock the IP by removing the DROP rule from iptables
// For allow rules, this will remove the explicit ALLOW rule (default behavior will apply)
func (m *Manager) DeleteRule(ruleID uint) error {
	// Get existing rule
	var rule models.FirewallRule
	if err := database.Instance().First(&rule, ruleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return errors.Errorf("firewall rule %d not found", ruleID)
		}
		return errors.Wrap(err, "failed to fetch firewall rule")
	}

	// Remove from iptables first (this will unblock if it's a block rule)
	if err := m.RemoveRule(&rule); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"rule_id":   rule.ID,
			"remote_ip": rule.RemoteIP,
			"port":      rule.ServerPort,
			"type":      rule.Type,
		}).Warn("failed to remove firewall rule from iptables during delete")
		// Continue with database deletion even if iptables removal fails
		// The rule should still be removed from database to keep them in sync
	} else {
		// Log successful removal with clear indication of what happened
		if rule.Type == models.FirewallRuleTypeBlock {
			log.WithFields(log.Fields{
				"rule_id":   rule.ID,
				"remote_ip": rule.RemoteIP,
				"port":      rule.ServerPort,
			}).Info("firewall block rule removed - IP is now unblocked")
		} else {
			log.WithFields(log.Fields{
				"rule_id":   rule.ID,
				"remote_ip": rule.RemoteIP,
				"port":      rule.ServerPort,
			}).Info("firewall allow rule removed - default firewall behavior will apply")
		}
	}

	// Delete from database (soft delete)
	if err := database.Instance().Delete(&rule).Error; err != nil {
		return errors.Wrap(err, "failed to delete firewall rule from database")
	}

	return nil
}

// GetRuleByID returns a single firewall rule by ID
func (m *Manager) GetRuleByID(ruleID uint) (*models.FirewallRule, error) {
	var rule models.FirewallRule
	if err := database.Instance().First(&rule, ruleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.Errorf("firewall rule %d not found", ruleID)
		}
		return nil, errors.Wrap(err, "failed to fetch firewall rule")
	}
	return &rule, nil
}

// GetRulesByPort returns all firewall rules for a specific port
func (m *Manager) GetRulesByPort(serverUUID string, port int) ([]models.FirewallRule, error) {
	var rules []models.FirewallRule
	if err := database.Instance().Where("server_uuid = ? AND server_port = ? AND deleted_at IS NULL", serverUUID, port).
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, errors.Wrap(err, "failed to fetch firewall rules")
	}
	return rules, nil
}

// CleanupInvalidPortRules removes firewall rules for ports that are no longer allocated to a server
func (m *Manager) CleanupInvalidPortRules(serverUUID string, validPorts map[int]bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get all rules for this server
	var rules []models.FirewallRule
	if err := database.Instance().Where("server_uuid = ? AND deleted_at IS NULL", serverUUID).Find(&rules).Error; err != nil {
		return errors.Wrap(err, "failed to fetch firewall rules for cleanup")
	}

	removedCount := 0
	for _, rule := range rules {
		// Check if the port is still valid
		if !validPorts[rule.ServerPort] {
			// Remove from iptables (lock already held, use unlocked version)
			if err := m.removeRuleUnlocked(&rule); err != nil {
				log.WithError(err).WithField("rule_id", rule.ID).Warn("failed to remove invalid firewall rule from iptables")
			}
			// Soft delete from database
			if err := database.Instance().Delete(&rule).Error; err != nil {
				log.WithError(err).WithField("rule_id", rule.ID).Warn("failed to delete invalid firewall rule from database")
			} else {
				removedCount++
			}
		}
	}

	if removedCount > 0 {
		log.WithFields(log.Fields{
			"server": serverUUID,
			"count":  removedCount,
		}).Info("cleaned up invalid firewall rules for server")
	}

	return nil
}

// DeleteAllRulesForServer deletes all firewall rules for a server
func (m *Manager) DeleteAllRulesForServer(serverUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get all rules for this server
	var rules []models.FirewallRule
	if err := database.Instance().Where("server_uuid = ? AND deleted_at IS NULL", serverUUID).Find(&rules).Error; err != nil {
		return errors.Wrap(err, "failed to fetch firewall rules for deletion")
	}

	// Remove each rule from iptables
	for _, rule := range rules {
		if err := m.RemoveRule(&rule); err != nil {
			log.WithError(err).WithField("rule_id", rule.ID).Warn("failed to remove firewall rule from iptables during server deletion")
		}
	}

	// Soft delete all rules from database
	if err := database.Instance().Where("server_uuid = ? AND deleted_at IS NULL", serverUUID).
		Delete(&models.FirewallRule{}).Error; err != nil {
		return errors.Wrap(err, "failed to delete firewall rules from database")
	}

	log.WithFields(log.Fields{
		"server": serverUUID,
		"count":  len(rules),
	}).Info("deleted all firewall rules for server")

	return nil
}

// RebuildAllRules rebuilds all firewall rules in iptables (useful for system restart)
func (m *Manager) RebuildAllRules() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get all active rules grouped by server
	var allRules []models.FirewallRule
	if err := database.Instance().Where("deleted_at IS NULL").
		Order("server_uuid ASC, priority ASC, created_at ASC").
		Find(&allRules).Error; err != nil {
		return errors.Wrap(err, "failed to fetch all firewall rules")
	}

	if len(allRules) == 0 {
		log.Debug("no firewall rules to rebuild")
		return nil
	}

	// Group by server
	serverRules := make(map[string][]models.FirewallRule)
	for _, rule := range allRules {
		serverRules[rule.ServerUUID] = append(serverRules[rule.ServerUUID], rule)
	}

	// Apply rules for each server (using unlocked version since we already have the lock)
	totalApplied := 0
	totalFailed := 0
	for serverUUID, rules := range serverRules {
		appliedCount := 0
		failedCount := 0
		for _, rule := range rules {
			if err := m.applyRuleUnlocked(&rule); err != nil {
				log.WithError(err).WithFields(log.Fields{
					"rule_id": rule.ID,
					"server":  serverUUID,
				}).Warn("failed to apply firewall rule during rebuild")
				failedCount++
			} else {
				appliedCount++
			}
		}
		totalApplied += appliedCount
		totalFailed += failedCount
		log.WithFields(log.Fields{
			"server":  serverUUID,
			"total":   len(rules),
			"applied": appliedCount,
			"failed":  failedCount,
		}).Info("rebuilt firewall rules")
	}

	log.WithFields(log.Fields{
		"total":   len(allRules),
		"applied": totalApplied,
		"failed":  totalFailed,
	}).Info("finished rebuilding all firewall rules")

	return nil
}

// ValidateIP validates an IP address or CIDR notation
func ValidateIP(ip string) error {
	if ip == "" {
		return errors.New("IP address cannot be empty")
	}

	// Try parsing as CIDR first
	_, _, err := net.ParseCIDR(ip)
	if err == nil {
		return nil
	}

	// Try parsing as regular IP
	if net.ParseIP(ip) != nil {
		return nil
	}

	return errors.Errorf("invalid IP address or CIDR: %s", ip)
}


