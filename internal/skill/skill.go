package skill

import (
	"path/filepath"
	"strings"

	"ai-bricklaying/internal/safeio"
)

type Config struct {
	ConfigPath       string
	Language         string
	OutputDir        string
	OutputModes      []string
	SkillName        string
	MetadataPath     string
	SlackPayloadPath string
	Sources          []Source
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

func Render(config Config) string {
	sourceNames := "none selected"
	if len(config.Sources) > 0 {
		names := make([]string, 0, len(config.Sources))
		for _, source := range config.Sources {
			names = append(names, source.Label)
		}
		sourceNames = strings.Join(names, ", ")
	}
	deliveryModes := strings.Join(config.OutputModes, ", ")
	outputLocationLines := []string{
		"- Summary directory: `" + config.OutputDir + "`",
		"- Metadata file: `" + config.MetadataPath + "`",
		"- Config file: `" + config.ConfigPath + "`",
	}
	if includes(config.OutputModes, "slack-webhook") {
		outputLocationLines = append(outputLocationLines, "- Slack payload file: `"+config.SlackPayloadPath+"`")
	} else {
		outputLocationLines = append(outputLocationLines, "- Slack payload file: not generated unless `slack-webhook` is selected")
	}

	deliveryInstructions := []string{
		"- `file`: always save the final markdown summary under `" + config.OutputDir + "` and report the saved path. Do not substitute the agent automation workspace unless the user explicitly asks for a different directory.",
	}
	if includes(config.OutputModes, "gmail-mcp") {
		deliveryInstructions = append(deliveryInstructions, "- `gmail-mcp`: when the CLI result includes this mode, prepare a Gmail MCP email draft handoff for the saved markdown summary using the configured recipient and subject. If recipient, subject, or authorization is missing, report the exact missing requirement instead of guessing.")
	}
	if includes(config.OutputModes, "slack-webhook") {
		deliveryInstructions = append(deliveryInstructions, "- `slack-webhook`: when the CLI result includes this mode, read `"+config.SlackPayloadPath+"` and post each entry in `messages` to the saved webhook from `"+config.ConfigPath+"`. Send the JSON blocks, not the raw Markdown text, so headings and lists render as Slack Block Kit. Report any missing webhook URL instead of exposing or inventing secrets.")
	}
	if len(config.OutputModes) == 1 && config.OutputModes[0] == "file" {
		deliveryInstructions = append(deliveryInstructions, "- File-only mode: do not attempt Gmail, Slack, or any external delivery unless the user explicitly asks for a new delivery mode later.")
	}

	slackPayloadContract := ""
	if includes(config.OutputModes, "slack-webhook") {
		slackPayloadContract = `
Slack payload content must mirror the saved Markdown summary:

- Treat the saved Markdown file as the source of truth for Slack delivery.
- Generate or update ` + "`ai-bricklaying-slack-payload.json`" + ` from the saved Markdown content after the Markdown file is finalized.
- Do not send a shortened, rewritten, or separately summarized Slack version unless the user explicitly asks for a Slack-specific summary.
- Preserve all Markdown sections and bullets in order. Convert headings and lists into Slack Block Kit ` + "`blocks`" + `; split long sections only to satisfy Slack block length limits.
- Before posting, verify that the Slack payload covers every top-level section in the saved Markdown summary.
`
	}

	markdown := `---
name: ` + config.SkillName + `
description: Summarize today's AI coding agent sessions into a useful compound-engineering briefing for the user.
---

# ` + config.SkillName + `

Use this skill when the user asks for a daily summary of AI coding work, session history, agent activity, or compound-engineering learnings.

## Sources

Default session sources: ` + sourceNames + `.

## Output Locations

` + strings.Join(outputLocationLines, "\n") + `

Use the summary directory above for final markdown files created by this skill. The configured CLI output directory is part of this skill's contract.

## CLI Result Delivery Modes

This skill was generated from the CLI result with delivery modes: ` + deliveryModes + `.

Follow the CLI-selected delivery modes exactly:

` + strings.Join(deliveryInstructions, "\n") + `
` + slackPayloadContract + `
## Workflow

1. Gather today's session history from the selected agents.
2. Identify actual work completed, decisions made, verification evidence, failed attempts, and reusable lessons.
3. Write the result in ` + config.Language + `.
4. Apply the CLI result delivery modes above: save the final markdown summary under the configured summary directory, then use only delivery modes selected in this generated skill.
5. Report saved files and delivery outcomes without printing secrets.

## Summary Template

` + renderTemplate(config.Language) + `
`
	return safeio.RedactString(markdown)
}

func Install(config Config, targets []Target) ([]string, error) {
	contents := []byte(Render(config))
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		path := filepath.Join(target.SkillDir, config.SkillName, "SKILL.md")
		if err := safeio.WriteFile(path, contents, safeio.WriteOptions{}); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
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

func includes(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
