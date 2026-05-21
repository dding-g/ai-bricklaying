// Package config owns persisted defaults and command-line override resolution.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StoredConfig mirrors the local config.json fields used as CLI defaults.
type StoredConfig struct {
	Delivery DeliveryConfig `json:"delivery"`
	Defaults DefaultsConfig `json:"defaults"`
}

// DeliveryConfig contains optional delivery defaults.
type DeliveryConfig struct {
	GmailRecipient  string `json:"gmail_recipient"`
	GmailSubject    string `json:"gmail_subject"`
	SlackWebhookURL string `json:"slack_webhook_url"`
}

// DefaultsConfig contains non-secret CLI defaults.
type DefaultsConfig struct {
	TargetAgents []string `json:"target_agents"`
	Source       string   `json:"source"`
	TargetModel  string   `json:"target_model"`
	Language     string   `json:"language"`
	OutputModes  []string `json:"output_modes"`
	SkillName    string   `json:"skill_name"`
	SkillDir     string   `json:"skill_dir"`
	OutputDir    string   `json:"output_dir"`
	CLIVersion   string   `json:"cli_version"`
}

// Load reads config.json from configDir. Missing files produce an empty config.
func Load(configDir string) (StoredConfig, string, error) {
	configPath := filepath.Join(ExpandHome(configDir), "config.json")
	contents, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return StoredConfig{}, configPath, nil
		}
		return StoredConfig{}, configPath, fmt.Errorf("Could not read config file %s: %w", configPath, err)
	}

	var stored StoredConfig
	if err := json.Unmarshal(contents, &stored); err != nil {
		return StoredConfig{}, configPath, fmt.Errorf("Could not read config file %s: %w", configPath, err)
	}

	return stored, configPath, nil
}

// ExpandHome resolves a leading ~ in the same way the Node CLI does for config paths.
func ExpandHome(value string) string {
	if value == "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Clean(value)
	}
	if value == "~" {
		return home
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	return filepath.Clean(value)
}

// SafeSummary returns a diagnostic-safe config summary without secret values.
func SafeSummary(stored StoredConfig) string {
	webhookStatus := "not configured"
	if stored.Delivery.SlackWebhookURL != "" {
		webhookStatus = "[configured]"
	}
	return fmt.Sprintf("targets=%s source=%s language=%s output_modes=%s slack_webhook_url=%s",
		strings.Join(stored.Defaults.TargetAgents, ","),
		stored.Defaults.Source,
		stored.Defaults.Language,
		strings.Join(stored.Defaults.OutputModes, ","),
		webhookStatus,
	)
}
