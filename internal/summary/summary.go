package summary

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"ai-bricklaying/internal/safeio"
	"ai-bricklaying/internal/sources"
)

const MetadataFileName = "ai-bricklaying-summary-skill.json"

var themeFamilies = []struct {
	name  string
	words []string
}{
	{name: "implementation", words: []string{"implement", "build", "ship", "feature"}},
	{name: "debugging", words: []string{"fix", "debug", "error", "bug", "fail"}},
	{name: "verification", words: []string{"test", "verify", "check", "review"}},
	{name: "planning", words: []string{"plan", "scope", "decide", "design"}},
	{name: "refactoring", words: []string{"refactor", "cleanup", "simplify"}},
	{name: "prompting", words: []string{"prompt", "skill", "agent", "session"}},
	{name: "learning", words: []string{"learn", "lesson", "insight", "improve"}},
}

type Source struct {
	Key   string
	Label string
}

type Target struct {
	Key      string
	Label    string
	SkillDir string
}

type Config struct {
	ConfigPath             string
	GmailRecipient         string
	GmailSubject           string
	Language               string
	CLIVersion             string
	OutputModes            []string
	OutputDir              string
	SkillName              string
	SlackWebhookConfigured bool
	SlackPayloadPath       string
	Source                 Source
	Targets                []Target
	TargetModel            string
}

type Metadata struct {
	ConfigPath             string   `json:"config_path"`
	Deliveries             []string `json:"deliveries"`
	GmailRecipient         *string  `json:"gmail_recipient"`
	GmailSubject           *string  `json:"gmail_subject"`
	Language               string   `json:"language"`
	CLIVersion             string   `json:"cli_version"`
	SessionCount           int      `json:"session_count"`
	Sessions               []string `json:"sessions"`
	SkillDirs              []string `json:"skill_dirs"`
	SlackWebhookConfigured bool     `json:"slack_webhook_configured"`
	SlackPayloadPath       *string  `json:"slack_payload_path"`
	SummaryPath            string   `json:"summary_path"`
	TargetAgents           []string `json:"target_agents"`
	TargetModel            string   `json:"target_model"`
}

func FileName(day time.Time) string {
	return fmt.Sprintf("%s-ai-bricklaying-daily-summary.md", localDateKey(day))
}

func BuildMarkdown(config Config, records []sources.Record, day time.Time) string {
	if day.IsZero() {
		day = time.Now()
	}
	lines := []string{
		fmt.Sprintf("# AI Bricklaying Daily Summary - %s", localDateKey(day)),
		"",
		fmt.Sprintf("Language: %s", config.Language),
		fmt.Sprintf("Target skill: %s", config.SkillName),
		fmt.Sprintf("Summary source: %s", config.Source.Label),
		"",
		"## Lightweight Session Signals",
	}

	if len(records) > 0 {
		lines = append(lines, fmt.Sprintf("- Found %d local session signal(s) from %s.", len(records), config.Source.Label))
		if themes := summarizeThemes(records); len(themes) > 0 {
			lines = append(lines, fmt.Sprintf("- Likely themes to reflect on: %s.", strings.Join(themes, ", ")))
		}
		lines = append(lines, "- Use the template below to turn those signals into a short lesson-focused reflection instead of an artifact log.")
	} else {
		lines = append(lines, "- No clear session signals were found today. Use the template below as a lightweight reflection prompt.")
	}

	lines = append(lines,
		"",
		"## Summary Template For AI Agent",
		"",
		renderTemplate(config.Language),
		"",
		"## Delivery Notes",
		"- File save: enabled",
	)
	if includes(config.OutputModes, "gmail-mcp") {
		recipient := nonempty(config.GmailRecipient, "not provided")
		subject := nonempty(config.GmailSubject, "not provided")
		lines = append(lines, fmt.Sprintf("- Gmail MCP: prepare an email draft for %s with subject %s", recipient, subject))
	}
	if includes(config.OutputModes, "slack-webhook") {
		status := "not provided"
		if config.SlackWebhookConfigured {
			status = "configured"
		}
		lines = append(lines, fmt.Sprintf("- Slack webhook URL: %s; use the saved config file for delivery", status))
	}

	return safeio.RedactString(strings.TrimSpace(strings.Join(lines, "\n")) + "\n")
}

func BuildMetadata(config Config, summaryPath string, sessionCount int) Metadata {
	skillDirs := make([]string, 0, len(config.Targets))
	targetAgents := make([]string, 0, len(config.Targets))
	for _, target := range config.Targets {
		skillDirs = append(skillDirs, filepath.Join(target.SkillDir, config.SkillName))
		targetAgents = append(targetAgents, target.Label)
	}
	metadata := Metadata{
		ConfigPath:             config.ConfigPath,
		Deliveries:             append([]string(nil), config.OutputModes...),
		GmailRecipient:         optional(config.GmailRecipient),
		GmailSubject:           optional(config.GmailSubject),
		Language:               config.Language,
		CLIVersion:             config.CLIVersion,
		SessionCount:           sessionCount,
		Sessions:               []string{config.Source.Key},
		SkillDirs:              skillDirs,
		SlackWebhookConfigured: config.SlackWebhookConfigured,
		SlackPayloadPath:       nil,
		SummaryPath:            summaryPath,
		TargetAgents:           targetAgents,
		TargetModel:            config.TargetModel,
	}
	if includes(config.OutputModes, "slack-webhook") && config.SlackPayloadPath != "" {
		metadata.SlackPayloadPath = optional(config.SlackPayloadPath)
	}
	return metadata
}

func renderTemplate(language string) string {
	return strings.ReplaceAll(`# Daily AI Bricklaying Summary

Write the summary in {language}.

Keep the summary lightweight and useful for the user. Do not list raw file paths, session artifact paths, or long evidence dumps unless the user explicitly asks for them.

## Today's Takeaways
Summarize the day in 3-5 short bullets. Focus on what changed in the user's understanding, workflow, or judgment.

## Lessons Learned
Capture reusable lessons: mistakes corrected, assumptions challenged, prompts that worked or failed, tool limits discovered, and decisions worth remembering.

## What Improved
Describe the practical improvements made today: process, product, code quality, documentation, verification habits, or agent workflow.

## Better AI Usage Next Time
Evaluate how the user worked with AI today. Suggest sharper prompts, better review habits, better delegation choices, or checks that would improve the next session.

## Tomorrow's Best Next Step
Give 1-3 concrete next actions. Keep them small, high-leverage, and easy to start.

## Follow-Up Prompt
Write one concise prompt the user can paste into an AI coding agent tomorrow. It should carry forward the lesson, not a long transcript.`, "{language}", language)
}

type themeCount struct {
	name  string
	count int
	order int
}

func summarizeThemes(records []sources.Record) []string {
	text := strings.ToLower(recordText(records))
	counts := make([]themeCount, 0, len(themeFamilies))
	for index, family := range themeFamilies {
		count := 0
		for _, word := range family.words {
			count += wordCount(text, word)
		}
		if count > 0 {
			counts = append(counts, themeCount{name: family.name, count: count, order: index})
		}
	}
	sort.SliceStable(counts, func(left, right int) bool {
		if counts[left].count == counts[right].count {
			return counts[left].order < counts[right].order
		}
		return counts[left].count > counts[right].count
	})
	limit := len(counts)
	if limit > 4 {
		limit = 4
	}
	result := make([]string, 0, limit)
	for _, item := range counts[:limit] {
		result = append(result, item.name)
	}
	return result
}

func recordText(records []sources.Record) string {
	parts := make([]string, 0, len(records))
	for _, record := range records {
		parts = append(parts, record.Text)
	}
	return strings.Join(parts, "\n")
}

func wordCount(text string, word string) int {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(word))
	return len(pattern.FindAllStringIndex(text, -1))
}

func localDateKey(day time.Time) string {
	return day.Local().Format("2006-01-02")
}

func includes(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func nonempty(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
