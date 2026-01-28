//go:build windows

package firewall

import (
	"github.com/apex/log"
	"github.com/priyxstudio/propel/internal/models"
)

// Windows firewall stub implementation.
// Docker Desktop on Windows handles port mapping, but specific firewall rules like blocking IPs
// are not implemented in this stub yet.

// ApplyRule applies a firewall rule. On Windows this is currently a no-op.
func (m *Manager) ApplyRule(rule *models.FirewallRule) error {
	log.Debug("firewall management is not currently supported on Windows (no-op)")
	return nil
}

// RemoveRule removes a firewall rule. On Windows this is currently a no-op.
func (m *Manager) RemoveRule(rule *models.FirewallRule) error {
	log.Debug("firewall management is not currently supported on Windows (no-op)")
	return nil
}

// applyRuleUnlocked is like ApplyRule but without locking.
func (m *Manager) applyRuleUnlocked(rule *models.FirewallRule) error {
	log.Debug("firewall management is not currently supported on Windows (no-op)")
	return nil
}

// removeRuleUnlocked removes a firewall rule without locking.
func (m *Manager) removeRuleUnlocked(rule *models.FirewallRule) error {
	log.Debug("firewall management is not currently supported on Windows (no-op)")
	return nil
}


