package cli

import (
	"bytes"
	"encoding/json"
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
			message: "--skill-name must be a path-safe slug",
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
	for _, expected := range []string{"## Sources", "## Output Locations", "## CLI Result Delivery Modes", "## Workflow", "## Summary Template", "File-only mode: do not attempt Gmail, Slack, or any external delivery", "This skill was generated from the CLI result with delivery modes: file."} {
		if !strings.Contains(skill, expected) {
			t.Fatalf("skill missing %q:\n%s", expected, skill)
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
	for _, expected := range []string{"[x] OpenCode", "[ ] Claude Code", "[x] File save (always enabled)", "AI Bricklaying files generated"} {
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
