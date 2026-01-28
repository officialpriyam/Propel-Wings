//go:build linux

package firewall

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/apex/log"

	"github.com/priyxstudio/propel/internal/database"
	"github.com/priyxstudio/propel/internal/models"
)

// executeIptables executes an iptables command with explicit arguments to prevent command injection
func (m *Manager) executeIptables(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "iptables", args...)

	// Capture stderr to include in error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = nil

	err := cmd.Run()
	if err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		cmdStr := strings.Join(append([]string{"iptables"}, args...), " ")

		if ctx.Err() == context.DeadlineExceeded {
			if stderrStr != "" {
				return errors.Wrapf(err, "iptables command timed out after 10 seconds: %s", stderrStr)
			}
			return errors.Wrap(err, "iptables command timed out after 10 seconds")
		}
		if ctx.Err() == context.Canceled {
			if stderrStr != "" {
				return errors.Wrapf(err, "iptables command was cancelled: %s", stderrStr)
			}
			return errors.Wrap(err, "iptables command was cancelled")
		}

		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ProcessState != nil {
				if !exitError.ProcessState.Exited() {
					if stderrStr != "" {
						return errors.Wrapf(err, "iptables command was killed (likely permission issue or system limit): %s", stderrStr)
					}
					return errors.Wrap(err, "iptables command was killed (likely permission issue or system limit)")
				}
				// Include the actual command and stderr in the error
				if stderrStr != "" {
					log.WithFields(log.Fields{
						"command": cmdStr,
						"stderr":  stderrStr,
						"exit":    exitError.ExitCode(),
					}).Error("iptables command failed")
					return errors.Wrapf(err, "iptables command failed (exit code: %d): %s", exitError.ExitCode(), stderrStr)
				}
				log.WithFields(log.Fields{
					"command": cmdStr,
					"exit":    exitError.ExitCode(),
				}).Error("iptables command failed")
				return errors.Wrapf(err, "iptables command failed (exit code: %d)", exitError.ExitCode())
			}
		}

		if stderrStr != "" {
			log.WithFields(log.Fields{
				"command": cmdStr,
				"stderr":  stderrStr,
			}).Error("iptables command failed")
			return errors.Wrapf(err, "iptables command failed: %s", stderrStr)
		}
		log.WithField("command", cmdStr).Error("iptables command failed")
		return errors.Wrap(err, "iptables command failed")
	}
	return nil
}

// buildIptablesRuleArgs builds iptables command arguments to prevent command injection
// Validates all inputs before building the command
func (m *Manager) buildIptablesRuleArgs(rule *models.FirewallRule, action string) ([]string, error) {
	// Validate IP address before use
	if err := ValidateIP(rule.RemoteIP); err != nil {
		return nil, errors.Wrap(err, "invalid remote IP")
	}

	// Validate and normalize protocol
	protocol := rule.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	if err := validateProtocol(protocol); err != nil {
		return nil, err
	}

	// Validate port
	if rule.ServerPort < 1 || rule.ServerPort > 65535 {
		return nil, errors.Errorf("invalid port: %d (must be between 1 and 65535)", rule.ServerPort)
	}

	// For Docker containers, traffic is DNAT'd in PREROUTING (nat table)
	// This means the destination port changes in FORWARD chain
	// We need to match on the original destination port BEFORE DNAT
	// Solution: Use PREROUTING in the raw table to match original port before DNAT
	table := "raw"
	chain := "PREROUTING"

	var target string
	if rule.Type == models.FirewallRuleTypeAllow {
		// For allow rules, we use ACCEPT to let the packet continue
		target = "ACCEPT"
	} else {
		// For blocking, use DROP to stop the packet before DNAT
		target = "DROP"
	}

	// Build arguments array
	args := []string{"-t", table}

	// For INSERT, try priority-based positioning, otherwise append
	if action == "-I" {
		// Calculate position based on priority
		position := m.calculateRulePosition(rule)
		if position > 0 && position == 1 {
			// Insert at the beginning (position 1)
			args = append(args, "-I", chain)
		} else if position > 1 {
			// Insert at specific position
			args = append(args, "-I", chain, fmt.Sprintf("%d", position))
		} else {
			// Position calculation failed, use append instead
			args = append(args, "-A", chain)
		}
	} else {
		// For other actions (like -D), use standard format
		args = append(args, action, chain)
	}

	// Add rule parameters
	args = append(args,
		"-p", protocol,
		"-s", rule.RemoteIP,
		"--dport", fmt.Sprintf("%d", rule.ServerPort),
		"-j", target,
	)

	return args, nil
}

// getChainLength gets the actual number of rules in the raw/PREROUTING chain from iptables
// We use raw/PREROUTING for both allow and block rules to match original destination port before DNAT
func (m *Manager) getChainLength(ruleType models.FirewallRuleType) (int, error) {
	table := "raw"
	chain := "PREROUTING"

	// Use -L with --line-numbers and count the lines (excluding headers)
	cmd := exec.Command("sh", "-c", fmt.Sprintf("iptables -t %s -L %s --line-numbers 2>/dev/null | tail -n +3 | wc -l", table, chain))
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil

	err := cmd.Run()
	if err != nil {
		return 0, errors.Wrap(err, "failed to get chain length")
	}

	output := strings.TrimSpace(stdout.String())
	length := 0
	fmt.Sscanf(output, "%d", &length)
	return length, nil
}

// calculateRulePosition calculates where to insert a rule based on its priority
// Returns 0 if we should append instead of insert at a specific position
func (m *Manager) calculateRulePosition(rule *models.FirewallRule) int {
	// Get actual chain length from iptables (for the appropriate table/chain based on rule type)
	chainLength, err := m.getChainLength(rule.Type)
	if err != nil {
		log.WithError(err).Debug("failed to get chain length, will append rule")
		return 0 // Append instead of insert
	}

	// If chain is empty, insert at position 1 (beginning)
	if chainLength == 0 {
		return 1
	}

	// Get all rules from database that are already applied (same server, same port, same protocol)
	// We only count rules that should be in iptables
	var existingRules []models.FirewallRule
	protocol := rule.Protocol
	if protocol == "" {
		protocol = "tcp"
	}

	database.Instance().Where("server_uuid = ? AND server_port = ? AND protocol = ? AND deleted_at IS NULL",
		rule.ServerUUID, rule.ServerPort, protocol).
		Order("priority ASC, created_at ASC").
		Find(&existingRules)

	// Count how many existing rules have priority <= our rule's priority
	// These should be inserted before our rule
	position := 1
	for _, r := range existingRules {
		// Skip the current rule if we're updating it
		if r.ID == rule.ID {
			continue
		}
		if r.Priority < rule.Priority || (r.Priority == rule.Priority && r.CreatedAt.Before(rule.CreatedAt)) {
			position++
		}
	}

	// Ensure position doesn't exceed chain length + 1 (for insertion at end)
	// If position calculation seems off, just append
	if position > chainLength+1 {
		log.WithFields(log.Fields{
			"calculated_position": position,
			"chain_length":        chainLength,
			"rule_id":             rule.ID,
			"protocol":            protocol,
		}).Debug("calculated position exceeds chain length, will append instead")
		return 0 // Append instead of insert
	}

	// If position is valid, return it
	if position >= 1 && position <= chainLength+1 {
		return position
	}

	// Fallback to append
	return 0
}

// ApplyRule applies a firewall rule to iptables
func (m *Manager) ApplyRule(rule *models.FirewallRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set default protocol if not set
	protocol := rule.Protocol
	if protocol == "" {
		protocol = "tcp"
	}

	target := map[models.FirewallRuleType]string{
		models.FirewallRuleTypeAllow: "ACCEPT",
		models.FirewallRuleTypeBlock: "DROP",
	}[rule.Type]

	// Validate inputs before building commands
	if err := ValidateIP(rule.RemoteIP); err != nil {
		return errors.Wrap(err, "invalid remote IP")
	}
	if err := validateProtocol(protocol); err != nil {
		return errors.Wrap(err, "invalid protocol")
	}

	// First, check if rule already exists in iptables (to avoid duplicates)
	// Use -C (check) which returns 0 if rule exists, 1 if it doesn't
	checkArgs := []string{
		"-t", "raw",
		"-C", "PREROUTING",
		"-p", protocol,
		"-s", rule.RemoteIP,
		"--dport", fmt.Sprintf("%d", rule.ServerPort),
		"-j", target,
	}

	// Execute check silently - we expect it to fail (exit code 1) if rule doesn't exist
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "iptables", checkArgs...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	checkErr := cmd.Run()
	if checkErr == nil {
		// Rule already exists, skip insertion
		log.WithFields(log.Fields{
			"rule_id":   rule.ID,
			"remote_ip": rule.RemoteIP,
			"port":      rule.ServerPort,
			"type":      rule.Type,
		}).Debug("firewall rule already exists in iptables, skipping")
		return nil
	}
	// If check failed (exit code 1), rule doesn't exist - this is expected, continue with insertion

	// Build and insert the rule with priority consideration
	insertArgs, err := m.buildIptablesRuleArgs(rule, "-I")
	if err != nil {
		return errors.Wrap(err, "failed to build iptables rule arguments")
	}

	log.WithFields(log.Fields{
		"rule_id": rule.ID,
		"command": strings.Join(append([]string{"iptables"}, insertArgs...), " "),
	}).Debug("applying firewall rule to iptables")

	if err := m.executeIptables(insertArgs...); err != nil {
		return errors.Wrapf(err, "failed to apply firewall rule %d", rule.ID)
	}

	log.WithFields(log.Fields{
		"rule_id":   rule.ID,
		"server":    rule.ServerUUID,
		"remote_ip": rule.RemoteIP,
		"port":      rule.ServerPort,
		"type":      rule.Type,
		"priority":  rule.Priority,
	}).Info("firewall rule applied")

	return nil
}

// RemoveRule removes a firewall rule from iptables
func (m *Manager) RemoveRule(rule *models.FirewallRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeRuleUnlocked(rule)
}

// removeRuleUnlocked removes a firewall rule from iptables without acquiring the lock
// This is used internally when the lock is already held
func (m *Manager) removeRuleUnlocked(rule *models.FirewallRule) error {
	// Validate inputs
	if err := ValidateIP(rule.RemoteIP); err != nil {
		return errors.Wrap(err, "invalid remote IP")
	}

	protocol := rule.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	if err := validateProtocol(protocol); err != nil {
		return errors.Wrap(err, "invalid protocol")
	}

	target := map[models.FirewallRuleType]string{
		models.FirewallRuleTypeAllow: "ACCEPT",
		models.FirewallRuleTypeBlock: "DROP",
	}[rule.Type]

	// We use raw table PREROUTING for both allow and block rules
	// Build delete command arguments
	deleteArgs := []string{
		"-t", "raw",
		"-D", "PREROUTING",
		"-p", protocol,
		"-s", rule.RemoteIP,
		"--dport", strconv.Itoa(rule.ServerPort),
		"-j", target,
	}

	if err := m.executeIptables(deleteArgs...); err != nil {
		// Log warning but don't fail - rule might not exist in iptables
		// This can happen if iptables was manually modified or rules were cleared
		log.WithError(err).WithFields(log.Fields{
			"rule_id":   rule.ID,
			"remote_ip": rule.RemoteIP,
			"port":      rule.ServerPort,
			"type":      rule.Type,
		}).Warn("failed to remove firewall rule from iptables (rule may not exist in iptables)")
		// Still return nil - the rule might not exist, which is fine
		return nil
	}

	// Rule successfully removed from iptables
	log.WithFields(log.Fields{
		"rule_id":   rule.ID,
		"remote_ip": rule.RemoteIP,
		"port":      rule.ServerPort,
		"type":      rule.Type,
	}).Debug("firewall rule removed from iptables")

	return nil
}

// applyRuleUnlocked is like ApplyRule but without locking (for use within locked contexts)
func (m *Manager) applyRuleUnlocked(rule *models.FirewallRule) error {
	// Set default protocol if not set
	protocol := rule.Protocol
	if protocol == "" {
		protocol = "tcp"
	}

	target := map[models.FirewallRuleType]string{
		models.FirewallRuleTypeAllow: "ACCEPT",
		models.FirewallRuleTypeBlock: "DROP",
	}[rule.Type]

	// First, check if rule already exists in iptables (to avoid duplicates)
	// Use -C (check) which returns 0 if rule exists, 1 if it doesn't
	// Redirect stderr to /dev/null to suppress the expected "Bad rule" message when rule doesn't exist
	// In DOCKER-USER, --dport matches the original host port
	checkCmd := fmt.Sprintf(
		"iptables -t filter -C DOCKER-USER -p %s -s %s --dport %d -j %s 2>/dev/null",
		protocol,
		rule.RemoteIP,
		rule.ServerPort,
		target,
	)

	// Execute check silently - we expect it to fail (exit code 1) if rule doesn't exist
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", checkCmd)
	cmd.Stdout = nil
	cmd.Stderr = nil

	checkErr := cmd.Run()
	if checkErr == nil {
		// Rule already exists, skip insertion
		log.WithFields(log.Fields{
			"rule_id":   rule.ID,
			"remote_ip": rule.RemoteIP,
			"port":      rule.ServerPort,
			"type":      rule.Type,
		}).Debug("firewall rule already exists in iptables, skipping")
		return nil
	}
	// If check failed (exit code 1), rule doesn't exist - this is expected, continue with insertion

	// Build and insert the rule with priority consideration
	insertArgs, err := m.buildIptablesRuleArgs(rule, "-I")
	if err != nil {
		return errors.Wrap(err, "failed to build iptables rule arguments")
	}

	log.WithFields(log.Fields{
		"rule_id": rule.ID,
		"command": strings.Join(append([]string{"iptables"}, insertArgs...), " "),
	}).Debug("applying firewall rule to iptables")

	if err := m.executeIptables(insertArgs...); err != nil {
		return errors.Wrapf(err, "failed to apply firewall rule %d", rule.ID)
	}

	log.WithFields(log.Fields{
		"rule_id":   rule.ID,
		"server":    rule.ServerUUID,
		"remote_ip": rule.RemoteIP,
		"port":      rule.ServerPort,
		"type":      rule.Type,
		"priority":  rule.Priority,
	}).Debug("firewall rule applied")

	return nil
}


