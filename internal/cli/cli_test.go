package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-bricklaying/internal/config"
)

func TestRunHelpAndVersionExitZeroWithoutArtifacts(t *testing.T) {
	root := t.TempDir()
	outputDir := filepath.Join(root, "out")
	configDir := filepath.Join(root, "config")

	for _, args := range [][]string{
		{"--help", "--output-dir", outputDir, "--config-dir", configDir},
		{"--version", "--output-dir", outputDir, "--config-dir", configDir},
		{"-h", "--output-dir", outputDir, "--config-dir", configDir},
		{"-v", "--output-dir", outputDir, "--config-dir", configDir},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run(args, &stdout, &stderr)

		if exitCode != 0 {
			t.Fatalf("Run(%v) exit code = %d, want 0; stderr=%q", args, exitCode, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("Run(%v) stderr = %q, want empty", args, stderr.String())
		}
		if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
			t.Fatalf("Run(%v) created output dir %s", args, outputDir)
		}
		if _, err := os.Stat(configDir); !os.IsNotExist(err) {
			t.Fatalf("Run(%v) created config dir %s", args, configDir)
		}
	}
}

func TestRunVersionAndGeneratedCliVersionIgnoreCallerPackageJSON(t *testing.T) {
	root := t.TempDir()
	callerDir := filepath.Join(root, "caller")
	if err := os.Mkdir(callerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(callerDir, "package.json"), []byte(`{"version":"9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(callerDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalCwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	var versionStdout bytes.Buffer
	var versionStderr bytes.Buffer
	versionExit := Run([]string{"--version"}, &versionStdout, &versionStderr)

	if versionExit != 0 {
		t.Fatalf("version exit code = %d, want 0; stderr=%q", versionExit, versionStderr.String())
	}
	if got := strings.TrimSpace(versionStdout.String()); got != "0.1.0" {
		t.Fatalf("version stdout = %q, want package version 0.1.0", got)
	}

	outputDir := filepath.Join(root, "out")
	configDir := filepath.Join(root, "config")
	var generateStdout bytes.Buffer
	var generateStderr bytes.Buffer
	generateExit := Run([]string{
		"--non-interactive",
		"--target-agent", "opencode",
		"--sources", "opencode",
		"--output-dir", outputDir,
		"--skill-dir", filepath.Join(root, "skills"),
		"--config-dir", configDir,
	}, &generateStdout, &generateStderr)

	if generateExit != 0 {
		t.Fatalf("generate exit code = %d, want 0; stderr=%q", generateExit, generateStderr.String())
	}
	metadataPath := filepath.Join(outputDir, "ai-bricklaying-summary-skill.json")
	var metadata map[string]any
	if err := json.Unmarshal([]byte(readFile(t, metadataPath)), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["cli_version"] != "0.1.0" {
		t.Fatalf("metadata cli_version = %#v, want 0.1.0", metadata["cli_version"])
	}
	var saved config.StoredConfig
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(configDir, "config.json"))), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Defaults.CLIVersion != "0.1.0" {
		t.Fatalf("config cli_version = %q, want 0.1.0", saved.Defaults.CLIVersion)
	}
}

func TestValidationErrorsExitTwoWithContractMessages(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{
			name:    "multiple sources",
			args:    []string{"--non-interactive", "--sources", "opencode,codex", "--config-dir", filepath.Join(t.TempDir(), "config")},
			message: "--sources accepts exactly one summary source",
		},
		{
			name:    "source outside target",
			args:    []string{"--non-interactive", "--target-agent", "opencode", "--sources", "codex", "--config-dir", filepath.Join(t.TempDir(), "config")},
			message: "--sources must be one of the selected target agents: opencode",
		},
		{
			name:    "missing slack webhook",
			args:    []string{"--non-interactive", "--output-modes", "slack-webhook", "--config-dir", filepath.Join(t.TempDir(), "config")},
			message: "slack-webhook requires --slack-webhook-url",
		},
		{
			name:    "unsafe skill name",
			args:    []string{"--non-interactive", "--skill-name", "../escape", "--config-dir", filepath.Join(t.TempDir(), "config")},
			message: "--skill-name must be 1-64 lowercase letters",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := Run(test.args, &stdout, &stderr)

			if exitCode != 2 {
				t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.message) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.message)
			}
		})
	}
}

func TestSkillNameFollowsAgentSkillsPortableSlugContract(t *testing.T) {
	for _, value := range []string{"a", "ai-bricklaying-worklog", "skill2"} {
		if err := validateSkillName(value); err != nil {
			t.Fatalf("valid skill name %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "team_log", "team.log", "-team", "team-", "team--log", strings.Repeat("a", 65)} {
		if err := validateSkillName(value); err == nil {
			t.Fatalf("non-portable skill name %q accepted", value)
		}
	}
}

func TestStoredSkillNameMigrationOnlyNormalizesLegacySafeNames(t *testing.T) {
	tests := map[string]string{
		"already-portable":      "already-portable",
		"legacy_summary":        "legacy-summary",
		"legacy.summary_name-":  "legacy-summary-name",
		"legacy..summary":       "legacy..summary",
		"../escape":             "../escape",
		"UPPER_CASE":            "UPPER_CASE",
		strings.Repeat("a", 65): strings.Repeat("a", 65),
	}
	for input, want := range tests {
		if got := migrateStoredSkillName(input); got != want {
			t.Errorf("migrateStoredSkillName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTargetCatalogUsesGitHubCopilotPersonalSkillDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("COPILOT_HOME", "")

	got := targetCatalog()["github-copilot"].DefaultSkillDir
	want := filepath.Join(home, ".copilot", "skills")
	if got != want {
		t.Fatalf("GitHub Copilot skill directory = %q, want %q", got, want)
	}
}

func TestSavedCopilotDefaultSkillDirectoryMigrationRespectsCopilotHomeAndCustomPaths(t *testing.T) {
	tests := []struct {
		name         string
		copilotHome  string
		savedPath    func(string) string
		expectedPath func(string, string) string
	}{
		{
			name:         "old product path moves to current home default",
			savedPath:    func(home string) string { return filepath.Join(home, ".github-copilot", "skills") },
			expectedPath: func(home string, _ string) string { return filepath.Join(home, ".copilot", "skills") },
		},
		{
			name:         "old product path moves to COPILOT_HOME",
			copilotHome:  "custom-copilot-home",
			savedPath:    func(home string) string { return filepath.Join(home, ".github-copilot", "skills") },
			expectedPath: func(_ string, copilotHome string) string { return filepath.Join(copilotHome, "skills") },
		},
		{
			name:         "home default moves to COPILOT_HOME",
			copilotHome:  "custom-copilot-home",
			savedPath:    func(home string) string { return filepath.Join(home, ".copilot", "skills") },
			expectedPath: func(_ string, copilotHome string) string { return filepath.Join(copilotHome, "skills") },
		},
		{
			name:         "custom saved path remains custom",
			copilotHome:  "custom-copilot-home",
			savedPath:    func(home string) string { return filepath.Join(home, "my-shared-skills") },
			expectedPath: func(home string, _ string) string { return filepath.Join(home, "my-shared-skills") },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			copilotHome := ""
			if test.copilotHome != "" {
				copilotHome = filepath.Join(home, test.copilotHome)
			}
			t.Setenv("COPILOT_HOME", copilotHome)
			configDir := filepath.Join(t.TempDir(), "config")
			if err := os.Mkdir(configDir, 0o700); err != nil {
				t.Fatal(err)
			}
			writeConfig(t, configDir, map[string]any{
				"defaults": map[string]any{
					"target_agents": []string{"github-copilot"},
					"source":        "github-copilot",
					"skill_dir":     test.savedPath(home),
				},
			})

			resolved, err := resolveFromArgs(t, "--non-interactive", "--config-dir", configDir)
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			want := test.expectedPath(home, copilotHome)
			if resolved.SkillDir != want || resolved.TargetAgents[0].DefaultSkillDir != want {
				t.Fatalf("resolved Copilot skill dir = %q / %q, want %q", resolved.SkillDir, resolved.TargetAgents[0].DefaultSkillDir, want)
			}
		})
	}
}

func TestRunPersistsSavedCopilotDefaultSkillDirectoryMigration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	copilotHome := filepath.Join(home, "copilot-profile")
	t.Setenv("COPILOT_HOME", copilotHome)
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, configDir, map[string]any{
		"defaults": map[string]any{
			"target_agents": []string{"github-copilot"},
			"source":        "github-copilot",
			"output_modes":  []string{"file"},
			"skill_name":    "copilot-path-migration",
			"skill_dir":     filepath.Join(home, ".github-copilot", "skills"),
			"output_dir":    filepath.Join(root, "out"),
		},
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run([]string{"--non-interactive", "--config-dir", configDir}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Run exit = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	want := filepath.Join(copilotHome, "skills")
	var saved config.StoredConfig
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(configDir, "config.json"))), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Defaults.SkillDir != want {
		t.Fatalf("persisted Copilot skill dir = %q, want %q", saved.Defaults.SkillDir, want)
	}
	if _, err := os.Stat(filepath.Join(want, "copilot-path-migration", "SKILL.md")); err != nil {
		t.Fatalf("migrated Copilot skill was not installed: %v", err)
	}
}

func TestExplicitSourceWithoutTargetPreservesLegacyTargetSelection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, configDir, map[string]any{
		"defaults": map[string]any{
			"target_agents": []string{"claude-code"},
			"skill_dir":     filepath.Join(home, ".claude", "skills"),
		},
	})

	resolved, err := resolveFromArgs(t,
		"--non-interactive",
		"--sources", "opencode",
		"--config-dir", configDir,
	)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got := strings.Join(targetKeys(resolved.TargetAgents), ","); got != "opencode" {
		t.Fatalf("targets = %q, want source-compatible opencode", got)
	}
	if resolved.Source.Key != "opencode" {
		t.Fatalf("source = %q, want opencode", resolved.Source.Key)
	}
	if resolved.SkillDir != "" {
		t.Fatalf("promoted source retained saved skill dir %q", resolved.SkillDir)
	}
	if got, want := resolved.TargetAgents[0].DefaultSkillDir, filepath.Join(home, ".config", "opencode", "skills"); got != want {
		t.Fatalf("promoted target skill dir = %q, want %q", got, want)
	}
}

func TestExplicitSourcePromotionPreservesExplicitSkillDir(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, configDir, map[string]any{
		"defaults": map[string]any{
			"target_agents": []string{"claude-code"},
			"skill_dir":     filepath.Join(root, "saved-claude-skills"),
		},
	})
	explicitDir := filepath.Join(root, "explicit-shared-skills")
	resolved, err := resolveFromArgs(t,
		"--non-interactive",
		"--sources", "opencode",
		"--skill-dir", explicitDir,
		"--config-dir", configDir,
	)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.SkillDir != explicitDir || resolved.TargetAgents[0].DefaultSkillDir != explicitDir {
		t.Fatalf("explicit skill dir was not preserved: %#v", resolved)
	}
}

func TestExplicitTargetSourceMismatchRemainsStrict(t *testing.T) {
	_, err := resolveFromArgs(t,
		"--non-interactive",
		"--target-agent", "claude-code",
		"--sources", "opencode",
		"--config-dir", filepath.Join(t.TempDir(), "config"),
	)
	if err == nil || !strings.Contains(err.Error(), "--sources must be one of the selected target agents: claude-code") {
		t.Fatalf("Resolve mismatch error = %v", err)
	}
}

func TestSourceOnlyCompatibilityKeepsUnknownSourceError(t *testing.T) {
	_, err := resolveFromArgs(t,
		"--non-interactive",
		"--sources", "unknown-agent",
		"--config-dir", filepath.Join(t.TempDir(), "config"),
	)
	if err == nil || !strings.Contains(err.Error(), "Unknown source(s): unknown-agent") {
		t.Fatalf("Resolve unknown source error = %v", err)
	}
}

func TestExplicitEmptySkillNameAndSourceShapesFailStrictValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{name: "inline empty skill name", args: []string{"--skill-name="}, message: "--skill-name must be 1-64"},
		{name: "separate empty skill name", args: []string{"--skill-name", ""}, message: "--skill-name must be 1-64"},
		{name: "inline empty source", args: []string{"--sources="}, message: "--sources accepts exactly one"},
		{name: "whitespace source", args: []string{"--sources=   "}, message: "--sources accepts exactly one"},
		{name: "commas only source", args: []string{"--sources=,,,"}, message: "--sources accepts exactly one"},
		{name: "trailing empty source", args: []string{"--sources=opencode,"}, message: "--sources accepts exactly one"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--non-interactive", "--config-dir", filepath.Join(t.TempDir(), "config")}, test.args...)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if exitCode := Run(args, &stdout, &stderr); exitCode != contractExit {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", exitCode, contractExit, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.message) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.message)
			}
		})
	}
}

func TestAliasesAndFileModeInvariant(t *testing.T) {
	root := t.TempDir()
	resolved, err := resolveFromArgs(t,
		"--non-interactive",
		"--target-agent", "opencode,codex",
		"--sessions", "codex",
		"--delivery", "gmail,slack",
		"--gmail-to", "team@example.com",
		"--gmail-subject", "Daily summary",
		"--slack-webhook-url", "https://hooks.slack.com/services/T000/B000/secret",
		"--config-dir", filepath.Join(root, "config"),
	)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if resolved.Source.Key != "codex" {
		t.Fatalf("source = %q, want codex", resolved.Source.Key)
	}
	if got, want := resolved.OutputModes, []string{"file", "gmail-mcp", "slack-webhook"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("output modes = %v, want %v", got, want)
	}
	if resolved.GmailRecipient != "team@example.com" {
		t.Fatalf("gmail recipient = %q", resolved.GmailRecipient)
	}
}

func TestConfigDefaultsApplyOnlyWhenFlagsMissing(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, configDir, map[string]any{
		"delivery": map[string]any{
			"gmail_recipient":   "saved@example.com",
			"gmail_subject":     "Saved subject",
			"slack_webhook_url": "https://hooks.slack.com/services/T000/B000/saved-secret",
		},
		"defaults": map[string]any{
			"target_agents": []string{"opencode", "codex"},
			"source":        "codex",
			"target_model":  "saved model",
			"language":      "Korean",
			"output_modes":  []string{"file", "slack-webhook"},
			"skill_name":    "saved-session-summary",
			"skill_dir":     filepath.Join(root, "saved-skills"),
			"output_dir":    filepath.Join(root, "saved-out"),
		},
	})

	resolved, err := resolveFromArgs(t,
		"--non-interactive",
		"--target-agent", "opencode",
		"--sources", "opencode",
		"--language", "English",
		"--output-modes", "file",
		"--skill-name", "flag-session-summary",
		"--config-dir", configDir,
	)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if got := targetKeys(resolved.TargetAgents); strings.Join(got, ",") != "opencode" {
		t.Fatalf("targets = %v, want flag target", got)
	}
	if resolved.Source.Key != "opencode" {
		t.Fatalf("source = %q, want flag source", resolved.Source.Key)
	}
	if resolved.Language != "English" {
		t.Fatalf("language = %q, want flag language", resolved.Language)
	}
	if strings.Join(resolved.OutputModes, ",") != "file" {
		t.Fatalf("output modes = %v, want file", resolved.OutputModes)
	}
	if resolved.SkillName != "flag-session-summary" {
		t.Fatalf("skill name = %q, want flag skill name", resolved.SkillName)
	}
	if resolved.GmailRecipient != "saved@example.com" || resolved.GmailSubject != "Saved subject" {
		t.Fatalf("delivery defaults were not applied: %#v", resolved)
	}
	if resolved.SlackWebhookURL != "https://hooks.slack.com/services/T000/B000/saved-secret" {
		t.Fatalf("saved webhook default missing")
	}
}

func TestLegacySavedSkillNameMigratesWithoutWeakeningExplicitFlag(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	skillDir := filepath.Join(root, "skills")
	outputDir := filepath.Join(root, "out")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, configDir, map[string]any{
		"defaults": map[string]any{
			"target_agents": []string{"opencode"},
			"source":        "opencode",
			"output_modes":  []string{"file"},
			"skill_name":    "legacy_summary",
			"skill_dir":     skillDir,
			"output_dir":    outputDir,
		},
	})

	resolved, err := resolveFromArgs(t, "--non-interactive", "--config-dir", configDir)
	if err != nil {
		t.Fatalf("Resolve returned error for legacy saved name: %v", err)
	}
	if resolved.SkillName != "legacy-summary" {
		t.Fatalf("migrated skill name = %q, want legacy-summary", resolved.SkillName)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run([]string{"--non-interactive", "--config-dir", configDir}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Run exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(skillDir, "legacy-summary", "SKILL.md")); err != nil {
		t.Fatalf("migrated skill was not installed: %v", err)
	}
	var saved config.StoredConfig
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(configDir, "config.json"))), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Defaults.SkillName != "legacy-summary" {
		t.Fatalf("persisted skill name = %q, want legacy-summary", saved.Defaults.SkillName)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := Run([]string{"--non-interactive", "--config-dir", configDir}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("managed skill rerun exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}

	_, err = resolveFromArgs(t,
		"--non-interactive",
		"--skill-name", "explicit_name",
		"--config-dir", configDir,
	)
	if err == nil || !strings.Contains(err.Error(), "--skill-name must be 1-64 lowercase letters") {
		t.Fatalf("explicit legacy-shaped skill name should remain invalid, got %v", err)
	}
}

func TestLegacySavedSkillNameMigrationRefusesUnownedNormalizedDestination(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	skillDir := filepath.Join(root, "skills")
	destination := filepath.Join(skillDir, "legacy-summary", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("---\nname: legacy-summary\ndescription: user-owned skill\n---\n\n# Keep me\n")
	if err := os.WriteFile(destination, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, configDir, map[string]any{
		"defaults": map[string]any{
			"target_agents": []string{"opencode"},
			"source":        "opencode",
			"output_modes":  []string{"file"},
			"skill_name":    "legacy_summary",
			"skill_dir":     skillDir,
			"output_dir":    filepath.Join(root, "out"),
		},
	})
	configBefore := readFile(t, filepath.Join(configDir, "config.json"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Run([]string{"--non-interactive", "--config-dir", configDir}, &stdout, &stderr); exitCode != contractExit {
		t.Fatalf("Run exit code = %d, want %d; stderr=%q", exitCode, contractExit, stderr.String())
	}
	if !strings.Contains(stderr.String(), "skill destination collision") {
		t.Fatalf("stderr missing collision guidance: %q", stderr.String())
	}
	if got, err := os.ReadFile(destination); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("unowned skill changed: err=%v contents=%q", err, got)
	}
	if got := readFile(t, filepath.Join(configDir, "config.json")); got != configBefore {
		t.Fatalf("config changed despite migration collision:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(root, "out")); !os.IsNotExist(err) {
		t.Fatalf("collision should fail in preflight before summary output, stat err=%v", err)
	}
}

func TestConfigDefaultsCanSatisfySlackWithoutLeakingSecret(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := "https://hooks.slack.com/services/T000/B000/saved-secret"
	writeConfig(t, configDir, map[string]any{
		"delivery": map[string]any{
			"slack_webhook_url": secret,
		},
		"defaults": map[string]any{
			"target_agents": []string{"opencode"},
			"source":        "opencode",
			"output_modes":  []string{"file", "slack-webhook"},
			"output_dir":    filepath.Join(root, "out"),
			"skill_dir":     filepath.Join(root, "skills"),
		},
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{"--non-interactive", "--config-dir", configDir}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if strings.Contains(stdout.String(), "saved-secret") || strings.Contains(stderr.String(), "saved-secret") {
		t.Fatalf("output leaked saved webhook: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "AI Bricklaying files generated") {
		t.Fatalf("stdout = %q, want artifact completion", stdout.String())
	}
}

func TestRunFileOnlyGeneratesSummaryMetadataConfigAndSkill(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.Mkdir(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "today.jsonl"), []byte(`{"message":"debugged implementation tests and improved agent review habits","password":"hunter2","token":"secret-token-value"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_BRICKLAYING_OPENCODE_DIRS", sessionDir)

	outputDir := filepath.Join(root, "out")
	skillDir := filepath.Join(root, "skills")
	configDir := filepath.Join(root, "config")
	skillName := "file-only-session-summary"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"--non-interactive",
		"--target-agent", "opencode",
		"--sources", "opencode",
		"--language", "Korean",
		"--output-modes", "file",
		"--skill-name", skillName,
		"--output-dir", outputDir,
		"--skill-dir", skillDir,
		"--config-dir", configDir,
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	summaryPath := filepath.Join(outputDir, localSummaryFileName())
	metadataPath := filepath.Join(outputDir, "ai-bricklaying-summary-skill.json")
	configPath := filepath.Join(configDir, "config.json")
	skillPath := filepath.Join(skillDir, skillName, "SKILL.md")
	for _, path := range []string{summaryPath, metadataPath, configPath, skillPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s should exist: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outputDir, "ai-bricklaying-slack-payload.json")); !os.IsNotExist(err) {
		t.Fatalf("slack payload should not exist in file-only mode")
	}

	summary := readFile(t, summaryPath)
	for _, expected := range []string{"AI Bricklaying Daily Summary", "Language: Korean", "Summary source: OpenCode", "Lightweight Session Signals", "Summary Template For AI Agent", "Delivery Notes", "Today's Takeaways", "Lessons Learned", "What Improved", "Better AI Usage Next Time", "Tomorrow's Best Next Step"} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("summary missing %q:\n%s", expected, summary)
		}
	}
	for _, forbidden := range []string{sessionDir, "hunter2", "secret-token-value"} {
		if strings.Contains(summary, forbidden) {
			t.Fatalf("summary leaked %q:\n%s", forbidden, summary)
		}
	}
	if !strings.Contains(summary, "debugging") {
		t.Fatalf("summary should include discovered debugging theme, got:\n%s", summary)
	}

	skill := readFile(t, skillPath)
	for _, expected := range []string{"## Sources", "## Output Locations", "## CLI Result Delivery Modes", "## Workflow", "File-only mode: do not attempt Gmail, Slack, or any external delivery", "This skill was generated from the CLI result with delivery modes: file."} {
		if !strings.Contains(skill, expected) {
			t.Fatalf("skill missing %q:\n%s", expected, skill)
		}
	}
	for _, sourceKey := range []string{"opencode", "claude-code", "codex", "cursor", "github-copilot"} {
		if !strings.Contains(skill, `"`+sourceKey+`"`) {
			t.Fatalf("worklog skill missing explicit source %q", sourceKey)
		}
	}
	for _, forbidden := range []string{"`gmail-mcp`: when", "`slack-webhook`: when", "Slack payload content must mirror"} {
		if strings.Contains(skill, forbidden) {
			t.Fatalf("file-only skill included external delivery instruction %q:\n%s", forbidden, skill)
		}
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(readFile(t, metadataPath)), &metadata); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(anyStringSlice(metadata["target_agents"]), ","); got != "OpenCode" {
		t.Fatalf("target_agents = %s, want OpenCode", got)
	}
	if got := strings.Join(anyStringSlice(metadata["sessions"]), ","); got != "opencode" {
		t.Fatalf("sessions = %s, want opencode", got)
	}
	if metadata["slack_payload_path"] != nil {
		t.Fatalf("slack_payload_path = %#v, want nil", metadata["slack_payload_path"])
	}

	var saved config.StoredConfig
	if err := json.Unmarshal([]byte(readFile(t, configPath)), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Defaults.CLIVersion != Version() {
		t.Fatalf("config cli_version = %q, want %q", saved.Defaults.CLIVersion, Version())
	}
	if saved.Defaults.SkillDir != skillDir {
		t.Fatalf("config skill_dir = %q, want %q", saved.Defaults.SkillDir, skillDir)
	}
	if !strings.Contains(stdout.String(), "Restart OpenCode or open a new session") {
		t.Fatalf("stdout missing OpenCode hint: %q", stdout.String())
	}
}

func TestRunSlackWebhookWritesPayloadOnlyWhenSelected(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.Mkdir(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "today.jsonl"), []byte(`{"message":"reviewed Slack payload with password=hunter2 and Bearer abcdef123456"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_BRICKLAYING_OPENCODE_DIRS", sessionDir)

	outputDir := filepath.Join(root, "out")
	skillDir := filepath.Join(root, "skills")
	configDir := filepath.Join(root, "config")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"--non-interactive",
		"--target-agent", "opencode",
		"--sources", "opencode",
		"--output-modes", "slack-webhook",
		"--slack-webhook-url", "https://hooks.slack.com/services/T000/B000/go-secret",
		"--output-dir", outputDir,
		"--skill-dir", skillDir,
		"--config-dir", configDir,
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if strings.Contains(stdout.String(), "go-secret") || strings.Contains(stderr.String(), "go-secret") {
		t.Fatalf("output leaked Slack secret: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	payloadPath := filepath.Join(outputDir, "ai-bricklaying-slack-payload.json")
	payloadBytes, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"go-secret", "hunter2", "abcdef123456"} {
		if strings.Contains(string(payloadBytes), forbidden) {
			t.Fatalf("Slack payload leaked %q:\n%s", forbidden, string(payloadBytes))
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"text", "blocks", "messages", "verification"} {
		if _, ok := payload[field]; !ok {
			t.Fatalf("payload missing %s: %#v", field, payload)
		}
	}
	verification, ok := payload["verification"].(map[string]any)
	if !ok {
		t.Fatalf("verification = %#v", payload["verification"])
	}
	if verification["source"] != "saved_markdown" || verification["all_top_level_sections_covered"] != true {
		t.Fatalf("verification = %#v", verification)
	}
	sections := anyStringSlice(verification["top_level_sections"])
	for _, expected := range []string{"Lightweight Session Signals", "Summary Template For AI Agent", "Delivery Notes"} {
		if !containsString(sections, expected) {
			t.Fatalf("sections = %v, want %q", sections, expected)
		}
	}
}

func TestRunGmailHandoffIsPreparationOnlyWhenSelected(t *testing.T) {
	root := t.TempDir()
	outputDir := filepath.Join(root, "out")
	skillDir := filepath.Join(root, "skills")
	configDir := filepath.Join(root, "config")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"--non-interactive",
		"--target-agent", "opencode",
		"--sources", "opencode",
		"--output-modes", "gmail-mcp",
		"--gmail-recipient", "team@example.com",
		"--gmail-subject", "AI session summary",
		"--output-dir", outputDir,
		"--skill-dir", skillDir,
		"--config-dir", configDir,
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(outputDir, "ai-bricklaying-slack-payload.json")); !os.IsNotExist(err) {
		t.Fatalf("slack payload should not exist for gmail-only mode")
	}
	summary := readFile(t, filepath.Join(outputDir, localSummaryFileName()))
	skillText := readFile(t, filepath.Join(skillDir, defaultSkill, "SKILL.md"))
	for _, text := range []string{summary, skillText, stdout.String()} {
		if !strings.Contains(text, "Gmail") {
			t.Fatalf("gmail handoff text missing from %q", text)
		}
	}
	if !strings.Contains(summary, "Gmail MCP: prepare an email draft for team@example.com with subject AI session summary") {
		t.Fatalf("summary missing preparation copy:\n%s", summary)
	}
	if !strings.Contains(skillText, "prepare a Gmail MCP email draft handoff") {
		t.Fatalf("skill missing preparation-only copy:\n%s", skillText)
	}
	if strings.Contains(skillText, "prepare or send") || strings.Contains(stdout.String(), " to send ") {
		t.Fatalf("gmail handoff should be preparation only: stdout=%q skill=%q", stdout.String(), skillText)
	}
}

func TestRunInteractiveLineModeDefaultsGenerateArtifacts(t *testing.T) {
	root := t.TempDir()
	outputDir := filepath.Join(root, "out")
	skillDir := filepath.Join(root, "skills")
	configDir := filepath.Join(root, "config")

	stdout, stderr, exitCode := runInteractiveWithInput(t, "\n\n\n\n\n", []string{
		"--output-dir", outputDir,
		"--skill-dir", skillDir,
		"--config-dir", configDir,
	})

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr)
	}
	for _, expected := range []string{"[ ] OpenCode", "[x] Claude Code", "[x] File save (always enabled)", "AI Bricklaying files generated"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("stdout missing %q:\n%s", expected, stdout)
		}
	}
	for _, path := range []string{
		filepath.Join(outputDir, localSummaryFileName()),
		filepath.Join(outputDir, "ai-bricklaying-summary-skill.json"),
		filepath.Join(skillDir, defaultSkill, "SKILL.md"),
		filepath.Join(configDir, "config.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s should exist: %v", path, err)
		}
	}
}

func TestRunInteractiveLineModeFiltersSourcesToSelectedTarget(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, exitCode := runInteractiveWithInput(t, "3\n\n\n\n\n", []string{
		"--output-dir", filepath.Join(root, "out"),
		"--skill-dir", filepath.Join(root, "skills"),
		"--config-dir", filepath.Join(root, "config"),
	})

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr)
	}
	sourceSection := stdout[strings.Index(stdout, "2. Select one AI agent whose sessions should be summarized"):strings.Index(stdout, "3. Result language")]
	if !strings.Contains(sourceSection, "Codex") {
		t.Fatalf("source section missing Codex:\n%s", sourceSection)
	}
	for _, forbidden := range []string{"OpenCode", "Claude Code"} {
		if strings.Contains(sourceSection, forbidden) {
			t.Fatalf("source section should not include %q:\n%s", forbidden, sourceSection)
		}
	}
}

func TestRunInteractiveLineModeShowsConfiguredSlackWebhookWithoutLeakingSecret(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := "https://hooks.slack.com/services/T000/B000/interactive-secret"
	writeConfig(t, configDir, map[string]any{
		"delivery": map[string]any{
			"slack_webhook_url": secret,
		},
		"defaults": map[string]any{
			"target_agents": []string{"opencode"},
			"source":        "opencode",
			"output_modes":  []string{"file", "slack-webhook"},
			"output_dir":    filepath.Join(root, "out"),
			"skill_dir":     filepath.Join(root, "skills"),
		},
	})

	stdout, stderr, exitCode := runInteractiveWithInput(t, "\n\n\n\n\n\n", []string{"--config-dir", configDir})

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr)
	}
	if !strings.Contains(stdout, "Slack webhook URL (optional) [configured]") {
		t.Fatalf("stdout missing configured Slack prompt:\n%s", stdout)
	}
	if strings.Contains(stdout, secret) || strings.Contains(stderr, secret) {
		t.Fatalf("output leaked Slack webhook: stdout=%q stderr=%q", stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "out", "ai-bricklaying-slack-payload.json")); err != nil {
		t.Fatalf("slack payload should exist: %v", err)
	}
}

func TestPromptSecretDoesNotEchoFreshValue(t *testing.T) {
	secret := "https://hooks.slack.com/services/T000/B000/fresh-interactive-secret"
	var stdout bytes.Buffer
	session := newPromptSession(&stdout, strings.NewReader(secret+"\n"))

	got, err := session.promptSecret("Slack webhook URL (optional)", "")
	if err != nil {
		t.Fatalf("promptSecret returned error: %v", err)
	}
	if got != secret {
		t.Fatalf("promptSecret returned %q, want supplied secret", got)
	}
	if strings.Contains(stdout.String(), secret) {
		t.Fatalf("fresh secret was echoed to stdout: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[hidden]") {
		t.Fatalf("stdout should acknowledge hidden input: %q", stdout.String())
	}
}

func TestPromptSecretUsesInjectedHiddenTerminalReader(t *testing.T) {
	secret := "https://hooks.slack.com/services/T000/B000/terminal-secret"
	var stdout bytes.Buffer
	called := false
	session := promptSession{
		stdout:  &stdout,
		scanner: bufio.NewScanner(strings.NewReader("echoed-fallback-secret\n")),
		secretReader: func() ([]byte, error) {
			called = true
			return []byte(secret), nil
		},
	}

	got, err := session.promptSecret("Slack webhook URL (optional)", "")
	if err != nil {
		t.Fatalf("promptSecret returned error: %v", err)
	}
	if !called {
		t.Fatal("promptSecret did not use the hidden terminal reader")
	}
	if got != secret {
		t.Fatalf("promptSecret returned %q, want supplied secret", got)
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stdout.String(), "echoed-fallback-secret") {
		t.Fatalf("hidden terminal input leaked to stdout: %q", stdout.String())
	}
}

func TestPromptSecretHiddenReaderFailureDoesNotFallBackToEchoedInput(t *testing.T) {
	var stdout bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader("must-not-be-read\n"))
	session := promptSession{
		stdout:       &stdout,
		scanner:      scanner,
		secretReader: func() ([]byte, error) { return nil, errors.New("terminal read failed") },
	}

	if _, err := session.promptSecret("Slack webhook URL (optional)", ""); err == nil {
		t.Fatal("promptSecret should fail closed when hidden terminal input fails")
	}
	if !scanner.Scan() || scanner.Text() != "must-not-be-read" {
		t.Fatal("promptSecret consumed the echoed fallback after hidden input failed")
	}
	if strings.Contains(stdout.String(), "must-not-be-read") {
		t.Fatalf("fallback secret leaked to stdout: %q", stdout.String())
	}
}

func TestPromptSecretTerminalCancelKeys(t *testing.T) {
	var stdout bytes.Buffer
	session := promptSession{
		stdout:       &stdout,
		scanner:      bufio.NewScanner(strings.NewReader("")),
		secretReader: func() ([]byte, error) { return nil, errSecretInputCancelled },
	}

	if _, err := session.promptSecret("Slack webhook URL (optional)", ""); err == nil || err.Error() != "Setup cancelled." {
		t.Fatalf("promptSecret cancel error = %v", err)
	}
	if !session.wasCancelled() {
		t.Fatal("promptSecret cancel did not mark the session cancelled")
	}
}

func TestPromptSecretNonTerminalFallbackKeepsLineInputContract(t *testing.T) {
	var stdout bytes.Buffer
	session := newPromptSession(&stdout, strings.NewReader("q\n"))

	got, err := session.promptSecret("Slack webhook URL (optional)", "")
	if err != nil {
		t.Fatalf("promptSecret returned error: %v", err)
	}
	if got != "q" {
		t.Fatalf("non-terminal fallback returned %q, want literal q", got)
	}
	if session.wasCancelled() {
		t.Fatal("non-terminal line fallback should not reinterpret q as a cancel key")
	}
}

func TestCompletionPathsStayOnOneRedactedLine(t *testing.T) {
	value := "/tmp/work\nnext\tBearer private-completion-token"
	output := completionPath(value)
	if strings.ContainsAny(output, "\r\n\t") {
		t.Fatalf("completion path contains line-breaking control: %q", output)
	}
	if strings.Contains(output, "private-completion-token") {
		t.Fatalf("completion path leaked credential-like text: %q", output)
	}
}

func TestTerminalPromptAccessibleKeysAndStandaloneEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "j", want: "down"},
		{input: "k", want: "up"},
		{input: "q", want: "cancel"},
		{input: "\x03", want: "ctrl-c"},
		{input: "\x1b", want: "cancel"},
		{input: "\x1b[A", want: "up"},
		{input: "\x1b[B", want: "down"},
	}
	for _, test := range tests {
		readPipe, writePipe, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writePipe.WriteString(test.input); err != nil {
			t.Fatal(err)
		}
		if err := writePipe.Close(); err != nil {
			t.Fatal(err)
		}
		prompt := terminalPrompt{stdin: readPipe}
		got := prompt.readKey()
		_ = readPipe.Close()
		if got != test.want {
			t.Errorf("readKey(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestTerminalPromptNoColorRenderingHasNoANSI(t *testing.T) {
	var stdout bytes.Buffer
	prompt := terminalPrompt{stdout: &stdout, noANSI: true}
	selected := map[int]bool{0: true}
	prompt.renderRows(0, "Choose", "Use j/k", []string{"One", "Two"}, selected, nil, 0)
	prompt.renderRows(5, "Choose", "Use j/k", []string{"One", "Two"}, selected, nil, 1)
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("NO_COLOR rendering emitted ANSI: %q", stdout.String())
	}
}

func TestRunInteractiveLineModeNoColorSuppressesANSI(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	root := t.TempDir()
	stdout, stderr, exitCode := runInteractiveWithInput(t, "\n\n\n\n\n", []string{
		"--output-dir", filepath.Join(root, "out"),
		"--skill-dir", filepath.Join(root, "skills"),
		"--config-dir", filepath.Join(root, "config"),
	})

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Fatalf("NO_COLOR stdout contained ANSI escape: %q", stdout)
	}
	for _, text := range []string{"Multi-select choices:", "Select one choice:", "Multi-select output modes:"} {
		if !strings.Contains(stdout, text) {
			t.Fatalf("stdout missing select-style prompt %q:\n%s", text, stdout)
		}
	}
}

func TestRunMultiTargetInstallsIdenticalSkillsAndMetadataArrays(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "sessions")
	if err := os.Mkdir(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "today.md"), []byte("implemented verification tests for generated skills"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("AI_BRICKLAYING_OPENCODE_DIRS", sessionDir)

	outputDir := filepath.Join(root, "out")
	configDir := filepath.Join(root, "config")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"--non-interactive",
		"--target-agent", "opencode,codex",
		"--sources", "opencode",
		"--output-dir", outputDir,
		"--config-dir", configDir,
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	opencodeSkillPath := filepath.Join(root, ".config", "opencode", "skills", defaultSkill, "SKILL.md")
	codexSkillPath := filepath.Join(root, ".codex", "skills", defaultSkill, "SKILL.md")
	opencodeSkill := readFile(t, opencodeSkillPath)
	codexSkill := readFile(t, codexSkillPath)
	if opencodeSkill != codexSkill {
		t.Fatalf("multi-target skill content should be identical")
	}

	metadataPath := filepath.Join(outputDir, "ai-bricklaying-summary-skill.json")
	var metadata map[string]any
	if err := json.Unmarshal([]byte(readFile(t, metadataPath)), &metadata); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(anyStringSlice(metadata["target_agents"]), ","); got != "OpenCode,Codex" {
		t.Fatalf("target_agents = %s, want OpenCode,Codex", got)
	}
	if got := strings.Join(anyStringSlice(metadata["sessions"]), ","); got != "opencode" {
		t.Fatalf("sessions = %s, want opencode", got)
	}
	skillDirs := anyStringSlice(metadata["skill_dirs"])
	if got := strings.Join(skillDirs, ","); got != filepath.Dir(opencodeSkillPath)+","+filepath.Dir(codexSkillPath) {
		t.Fatalf("skill_dirs = %v", skillDirs)
	}
	if !strings.Contains(stdout.String(), "Restart OpenCode or open a new session") {
		t.Fatalf("stdout missing OpenCode hint: %q", stdout.String())
	}
}

func runInteractiveWithInput(t *testing.T, input string, args []string) (string, string, int) {
	t.Helper()
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writePipe.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if err := writePipe.Close(); err != nil {
		t.Fatal(err)
	}
	originalStdin := os.Stdin
	os.Stdin = readPipe
	defer func() {
		os.Stdin = originalStdin
		readPipe.Close()
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func resolveFromArgs(t *testing.T, argv ...string) (ResolvedConfig, error) {
	t.Helper()
	args, err := ParseArgs(argv)
	if err != nil {
		return ResolvedConfig{}, err
	}
	return Resolve(args)
}

func localSummaryFileName() string {
	now := time.Now()
	return now.Format("2006-01-02") + "-ai-bricklaying-daily-summary.md"
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func anyStringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func writeConfig(t *testing.T, dir string, value map[string]any) {
	t.Helper()
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
}
