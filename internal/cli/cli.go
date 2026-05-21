// Package cli owns the command-line application boundary.
package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"golang.org/x/term"

	"ai-bricklaying/internal/config"
	"ai-bricklaying/internal/safeio"
	"ai-bricklaying/internal/skill"
	"ai-bricklaying/internal/slack"
	"ai-bricklaying/internal/sources"
	"ai-bricklaying/internal/summary"
)

const (
	defaultLanguage = "English"
	defaultModel    = "configured model"
	defaultSkill    = "daily-ai-session-summary"
	defaultTarget   = "opencode"
	contractExit    = 2
)

var (
	cliVersion      = "0.1.0"
	outputModeOrder = []string{"file", "gmail-mcp", "slack-webhook"}
	skillNameRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
)

// ParsedArgs contains raw flag values before config defaults are applied.
type ParsedArgs struct {
	Provided        map[string]bool
	NonInteractive  bool
	Help            bool
	Version         bool
	TargetAgent     string
	TargetModel     string
	Sources         string
	Language        string
	OutputModes     string
	SkillName       string
	SkillDir        string
	OutputDir       string
	GmailRecipient  string
	GmailSubject    string
	SlackWebhookURL string
	ConfigDir       string
}

type kongArgs struct {
	NonInteractive  bool   `name:"non-interactive" help:"Use defaults and flags without prompting."`
	TargetAgent     string `name:"target-agent" help:"Skill targets: opencode,claude-code,codex,cursor,github-copilot."`
	TargetModel     string `name:"target-model" help:"Model label recorded in generated artifacts."`
	Sources         string `name:"sources" aliases:"sessions" help:"Single session source to summarize."`
	Language        string `name:"language" help:"Language for the generated summary."`
	OutputModes     string `name:"output-modes" aliases:"delivery" help:"Output modes: file, gmail-mcp, slack-webhook."`
	SkillName       string `name:"skill-name" help:"Generated skill directory name."`
	SkillDir        string `name:"skill-dir" help:"Directory where the skill folder is written."`
	OutputDir       string `name:"output-dir" help:"Directory for summary files."`
	GmailRecipient  string `name:"gmail-recipient" aliases:"gmail-to" help:"Gmail MCP recipient."`
	GmailSubject    string `name:"gmail-subject" help:"Gmail MCP subject."`
	SlackWebhookURL string `name:"slack-webhook-url" help:"Slack incoming webhook URL."`
	ConfigDir       string `name:"config-dir" help:"ai-bricklaying config directory."`
}

// ResolvedConfig is the validated CLI configuration produced by flags plus saved defaults.
type ResolvedConfig struct {
	NonInteractive  bool
	TargetAgents    []Target
	TargetModel     string
	Source          Source
	Language        string
	OutputModes     []string
	SkillName       string
	SkillDir        string
	OutputDir       string
	GmailRecipient  string
	GmailSubject    string
	SlackWebhookURL string
	ConfigDir       string
	ConfigPath      string
}

// Target is a supported generated skill installation target.
type Target struct {
	Key             string
	Label           string
	DefaultSkillDir string
	ModelHint       string
}

// Source is a supported local session source.
type Source struct {
	Key   string
	Label string
}

type cliError struct {
	message string
}

func (err cliError) Error() string { return err.message }

// Run is the internal CLI entry point used by cmd/ai-bricklaying.
func Run(argv []string, stdout io.Writer, stderr io.Writer) int {
	args, err := ParseArgs(argv)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return contractExit
	}

	if args.Help {
		fmt.Fprint(stdout, HelpText())
		return 0
	}
	if args.Version {
		fmt.Fprintln(stdout, Version())
		return 0
	}

	resolved, err := Resolve(args)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return contractExit
	}

	if !resolved.NonInteractive {
		interactiveResolved, err := promptInteractive(resolved, stdout, os.Stdin)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return contractExit
		}
		resolved = interactiveResolved
	}

	if _, err := Generate(resolved, stdout); err != nil {
		if errors.Is(err, safeio.ErrSymlinkTarget) {
			fmt.Fprintln(stderr, err.Error())
			return contractExit
		}
		fmt.Fprintf(stderr, "Unexpected error: %s\n", err.Error())
		return 1
	}
	return 0
}

// ParseArgs parses documented flags and aliases without reading the filesystem.
func ParseArgs(argv []string) (ParsedArgs, error) {
	args := defaultParsedArgs()
	if infoArgs, ok := parseInformationArgs(argv, args); ok {
		return infoArgs, nil
	}
	if err := validateFlagShape(argv, &args); err != nil {
		return ParsedArgs{}, err
	}

	grammar := kongArgs{
		TargetModel: defaultModel,
		Language:    defaultLanguage,
		OutputModes: "file",
		SkillName:   defaultSkill,
		OutputDir:   defaultOutputDir(),
		ConfigDir:   defaultConfigDir(),
	}
	parser, err := kong.New(&grammar,
		kong.Name("ai-bricklaying"),
		kong.Description("Summarize today's AI agent sessions and generate a reusable skill."),
		kong.NoDefaultHelp(),
	)
	if err != nil {
		return ParsedArgs{}, err
	}
	if _, err := parser.Parse(argv); err != nil {
		return ParsedArgs{}, cliError{message: err.Error()}
	}
	applyKongDefaults(&grammar)

	args.NonInteractive = grammar.NonInteractive
	args.TargetAgent = grammar.TargetAgent
	args.TargetModel = grammar.TargetModel
	args.Sources = grammar.Sources
	args.Language = grammar.Language
	args.OutputModes = grammar.OutputModes
	args.SkillName = grammar.SkillName
	args.SkillDir = grammar.SkillDir
	args.OutputDir = grammar.OutputDir
	args.GmailRecipient = grammar.GmailRecipient
	args.GmailSubject = grammar.GmailSubject
	args.SlackWebhookURL = grammar.SlackWebhookURL
	args.ConfigDir = grammar.ConfigDir

	return args, nil
}

func applyKongDefaults(args *kongArgs) {
	if args.TargetModel == "" {
		args.TargetModel = defaultModel
	}
	if args.Language == "" {
		args.Language = defaultLanguage
	}
	if args.OutputModes == "" {
		args.OutputModes = "file"
	}
	if args.SkillName == "" {
		args.SkillName = defaultSkill
	}
	if args.OutputDir == "" {
		args.OutputDir = defaultOutputDir()
	}
	if args.ConfigDir == "" {
		args.ConfigDir = defaultConfigDir()
	}
}

func defaultParsedArgs() ParsedArgs {
	return ParsedArgs{
		Provided:     map[string]bool{},
		TargetModel:  defaultModel,
		Language:     defaultLanguage,
		OutputModes:  "file",
		SkillName:    defaultSkill,
		OutputDir:    defaultOutputDir(),
		ConfigDir:    defaultConfigDir(),
		TargetAgent:  "",
		Sources:      "",
		SkillDir:     "",
		GmailSubject: "",
	}
}

func parseInformationArgs(argv []string, args ParsedArgs) (ParsedArgs, bool) {
	for _, raw := range argv {
		flag, _, _ := strings.Cut(raw, "=")
		switch flag {
		case "--help", "-h":
			args.Help = true
			return args, true
		case "--version", "-v":
			args.Version = true
			return args, true
		}

	}
	return ParsedArgs{}, false
}

func validateFlagShape(argv []string, args *ParsedArgs) error {
	aliases := map[string]string{
		"--sessions": "--sources",
		"--delivery": "--output-modes",
		"--gmail-to": "--gmail-recipient",
	}
	keyMap := map[string]string{
		"--target-agent":      "targetAgent",
		"--target-model":      "targetModel",
		"--sources":           "sources",
		"--language":          "language",
		"--output-modes":      "outputModes",
		"--skill-name":        "skillName",
		"--skill-dir":         "skillDir",
		"--output-dir":        "outputDir",
		"--gmail-recipient":   "gmailRecipient",
		"--gmail-subject":     "gmailSubject",
		"--slack-webhook-url": "slackWebhookUrl",
		"--config-dir":        "configDir",
	}

	for index := 0; index < len(argv); index++ {
		raw := argv[index]
		flagPart, _, hasInline := strings.Cut(raw, "=")
		flag := flagPart
		if alias, ok := aliases[flagPart]; ok {
			flag = alias
		}

		switch flag {
		case "--non-interactive":
			args.Provided["nonInteractive"] = true
			continue
		}

		providedKey, ok := keyMap[flag]
		if !ok {
			return cliError{message: fmt.Sprintf("Unknown argument: %s", raw)}
		}

		if !hasInline {
			if index+1 >= len(argv) || strings.HasPrefix(argv[index+1], "--") {
				return cliError{message: fmt.Sprintf("%s requires a value", flag)}
			}
			index++
		}
		args.Provided[providedKey] = true
	}

	return nil
}

// Resolve applies saved config defaults and validates the executable contract.
func Resolve(args ParsedArgs) (ResolvedConfig, error) {
	args.ConfigDir = absolutePath(config.ExpandHome(args.ConfigDir))
	stored, configPath, err := config.Load(args.ConfigDir)
	if err != nil {
		return ResolvedConfig{}, cliError{message: err.Error()}
	}
	args = ApplyConfigDefaults(args, stored)

	targets, err := targetsFromArgs(args.TargetAgent, args.TargetModel)
	if err != nil {
		return ResolvedConfig{}, err
	}
	source, err := sourceFromArgs(args.Sources, targets)
	if err != nil {
		return ResolvedConfig{}, err
	}
	outputModes, err := outputModesFromArgs(args)
	if err != nil {
		return ResolvedConfig{}, err
	}
	if err := validateSkillName(args.SkillName); err != nil {
		return ResolvedConfig{}, err
	}
	args.OutputDir = absolutePath(config.ExpandHome(args.OutputDir))
	if args.SkillDir != "" {
		args.SkillDir = absolutePath(config.ExpandHome(args.SkillDir))
		for index := range targets {
			targets[index].DefaultSkillDir = args.SkillDir
		}
	}

	return ResolvedConfig{
		NonInteractive:  args.NonInteractive,
		TargetAgents:    targets,
		TargetModel:     args.TargetModel,
		Source:          source,
		Language:        args.Language,
		OutputModes:     outputModes,
		SkillName:       args.SkillName,
		SkillDir:        args.SkillDir,
		OutputDir:       args.OutputDir,
		GmailRecipient:  args.GmailRecipient,
		GmailSubject:    args.GmailSubject,
		SlackWebhookURL: args.SlackWebhookURL,
		ConfigDir:       args.ConfigDir,
		ConfigPath:      configPath,
	}, nil
}

// ApplyConfigDefaults fills only values not provided on the command line.
func ApplyConfigDefaults(args ParsedArgs, stored config.StoredConfig) ParsedArgs {
	provided := args.Provided
	defaults := stored.Defaults
	delivery := stored.Delivery

	if !provided["targetAgent"] && len(defaults.TargetAgents) > 0 {
		args.TargetAgent = strings.Join(defaults.TargetAgents, ",")
	}
	if !provided["sources"] && defaults.Source != "" {
		args.Sources = defaults.Source
	}
	if !provided["targetModel"] && defaults.TargetModel != "" {
		args.TargetModel = defaults.TargetModel
	}
	if !provided["language"] && defaults.Language != "" {
		args.Language = defaults.Language
	}
	if !provided["outputModes"] && len(defaults.OutputModes) > 0 {
		args.OutputModes = strings.Join(defaults.OutputModes, ",")
	}
	if !provided["skillName"] && defaults.SkillName != "" {
		args.SkillName = defaults.SkillName
	}
	if !provided["skillDir"] && defaults.SkillDir != "" {
		args.SkillDir = defaults.SkillDir
	}
	if !provided["outputDir"] && defaults.OutputDir != "" {
		args.OutputDir = defaults.OutputDir
	}
	if !provided["gmailRecipient"] && delivery.GmailRecipient != "" {
		args.GmailRecipient = delivery.GmailRecipient
	}
	if !provided["gmailSubject"] && delivery.GmailSubject != "" {
		args.GmailSubject = delivery.GmailSubject
	}
	if !provided["slackWebhookUrl"] && delivery.SlackWebhookURL != "" {
		args.SlackWebhookURL = delivery.SlackWebhookURL
	}

	return args
}

// HelpText returns the documented CLI help text.
func HelpText() string {
	return `Summarize today's AI agent sessions and generate a reusable skill.

Usage:
  ai-bricklaying [options]

Options:
  --non-interactive                 Use defaults and flags without prompting
  --target-agent <agents>           Skill targets: opencode,claude-code,codex,cursor,github-copilot
  --target-model <label>            Model label recorded in generated artifacts
  --sources, --sessions <source>    Single session source to summarize
  --language <language>             Language for the generated summary [English]
  --output-modes, --delivery <list> file, gmail-mcp, slack-webhook
  --skill-name <slug>               Generated skill directory name
  --skill-dir <dir>                 Directory where the skill folder is written
  --output-dir <dir>                Directory for summary files [~/ai-bricklaying]
  --gmail-recipient, --gmail-to     Gmail MCP recipient
  --gmail-subject <subject>         Gmail MCP subject
  --slack-webhook-url <url>         Slack incoming webhook URL
  --config-dir <dir>                ai-bricklaying config directory
  -v, --version                     Show CLI version
  -h, --help                        Show this help
`
}

// Version returns the package version for direct Go CLI information output.
func Version() string {
	return cliVersion
}

type promptSession struct {
	stdout   io.Writer
	scanner  *bufio.Scanner
	terminal *terminalPrompt
}

type terminalPrompt struct {
	stdin  *os.File
	stdout io.Writer
}

func promptInteractive(resolved ResolvedConfig, stdout io.Writer, stdin io.Reader) (ResolvedConfig, error) {
	session := newPromptSession(stdout, stdin)
	allTargets := targetOptions()
	if skillDir := resolvedInteractiveSkillDir(resolved); skillDir != "" {
		for index := range allTargets {
			allTargets[index].DefaultSkillDir = skillDir
		}
	}
	targets := session.chooseTargets("1. Select target AI agents for skill installation", allTargets, targetKeys(resolved.TargetAgents))
	resolved.TargetAgents = targets

	sourceOptions := sourceOptionsForTargets(targets)
	resolved.Source = session.chooseSource("2. Select one AI agent whose sessions should be summarized", sourceOptions, resolved.Source.Key)
	resolved.Language = session.promptLine("3. Result language", resolved.Language)
	resolved.OutputDir = absolutePath(config.ExpandHome(session.promptLine("4. File save directory", resolved.OutputDir)))
	resolved.OutputModes = session.chooseOutputModes("5. Select output modes", resolved.OutputModes)
	if includesMode(resolved.OutputModes, "gmail-mcp") {
		resolved.GmailRecipient = session.promptLine("Gmail recipient (optional)", resolved.GmailRecipient)
		resolved.GmailSubject = session.promptLine("Gmail subject (optional)", resolved.GmailSubject)
	}
	if includesMode(resolved.OutputModes, "slack-webhook") {
		resolved.SlackWebhookURL = session.promptSecret("Slack webhook URL (optional)", resolved.SlackWebhookURL)
	}
	return resolved, nil
}

func newPromptSession(stdout io.Writer, stdin io.Reader) promptSession {
	session := promptSession{stdout: stdout, scanner: bufio.NewScanner(stdin)}
	stdinFile, stdinOK := stdin.(*os.File)
	stdoutFile, stdoutOK := stdout.(*os.File)
	if stdinOK && stdoutOK && term.IsTerminal(int(stdinFile.Fd())) && term.IsTerminal(int(stdoutFile.Fd())) {
		session.terminal = &terminalPrompt{stdin: stdinFile, stdout: stdout}
	}
	return session
}

func resolvedInteractiveSkillDir(resolved ResolvedConfig) string {
	if resolved.SkillDir != "" {
		return resolved.SkillDir
	}
	return sharedSkillDir(resolved.TargetAgents)
}

func (session *promptSession) chooseTargets(title string, options []Target, defaultKeys []string) []Target {
	defaultIndexes := targetIndexes(options, defaultKeys)
	if len(defaultIndexes) == 0 {
		defaultIndexes = []int{0}
	}
	if session.terminal != nil {
		labels := make([]string, 0, len(options))
		for _, option := range options {
			labels = append(labels, option.Label)
		}
		selectedIndexes := session.terminal.selectRows(title, "Use ↑/↓ to move, Space to toggle, Enter to confirm.", labels, defaultIndexes, nil, true)
		selected := make([]Target, 0, len(selectedIndexes))
		for _, index := range selectedIndexes {
			selected = append(selected, options[index])
		}
		if len(selected) > 0 {
			return selected
		}
	}
	fmt.Fprintf(session.stdout, "\n%s\n", title)
	fmt.Fprintln(session.stdout, "Multi-select choices:")
	for index, option := range options {
		marker := "[ ]"
		if containsInt(defaultIndexes, index) {
			marker = "[x]"
		}
		fmt.Fprintf(session.stdout, "  %d) %s %s\n", index+1, marker, option.Label)
	}
	fmt.Fprintln(session.stdout, "Tip: leave blank for the shown defaults, or enter comma-separated numbers like 1,3,5.")
	answer := session.promptLine("Select numbers", indexesAnswer(defaultIndexes))
	selected := targetsByAnswer(options, answer)
	if len(selected) > 0 {
		return selected
	}
	selected = make([]Target, 0, len(defaultIndexes))
	for _, index := range defaultIndexes {
		if index >= 0 && index < len(options) {
			selected = append(selected, options[index])
		}
	}
	if len(selected) == 0 && len(options) > 0 {
		return []Target{options[0]}
	}
	return selected
}

func (session *promptSession) chooseSource(title string, options []Source, defaultKey string) Source {
	defaultIndex := 0
	for index, option := range options {
		if option.Key == defaultKey {
			defaultIndex = index
			break
		}
	}
	if session.terminal != nil {
		labels := make([]string, 0, len(options))
		for _, option := range options {
			labels = append(labels, option.Label)
		}
		selected := session.terminal.selectRows(title, "Use ↑/↓ to move, Enter to select.", labels, []int{defaultIndex}, nil, false)
		if len(selected) > 0 {
			return options[selected[0]]
		}
	}
	fmt.Fprintf(session.stdout, "\n%s\n", title)
	fmt.Fprintln(session.stdout, "Select one choice:")
	for index, option := range options {
		marker := "[ ]"
		if index == defaultIndex {
			marker = "[x]"
		}
		fmt.Fprintf(session.stdout, "  %d) %s %s\n", index+1, marker, option.Label)
	}
	for {
		answer := session.promptLine("Select number", fmt.Sprintf("%d", defaultIndex+1))
		number, ok := parsePositiveIndex(answer, len(options))
		if ok {
			return options[number]
		}
		fmt.Fprintln(session.stdout, "Enter one of the listed numbers.")
	}
}

func (session *promptSession) chooseOutputModes(title string, defaultModes []string) []string {
	defaultSet := map[string]bool{}
	for _, mode := range defaultModes {
		defaultSet[mode] = true
	}
	defaultIndexes := []int{0}
	for index, mode := range outputModeOrder {
		if index > 0 && defaultSet[mode] {
			defaultIndexes = append(defaultIndexes, index)
		}
	}
	labels := map[string]string{
		"file":          "File save (always enabled)",
		"gmail-mcp":     "Gmail MCP delivery notes",
		"slack-webhook": "Slack webhook config",
	}
	if session.terminal != nil {
		rowLabels := make([]string, 0, len(outputModeOrder))
		for _, mode := range outputModeOrder {
			label := labels[mode]
			if mode == "file" {
				label += " - required"
			}
			rowLabels = append(rowLabels, label)
		}
		selectedIndexes := session.terminal.selectRows(title, "Use ↑/↓ to move, Space to toggle optional modes, Enter to confirm.", rowLabels, defaultIndexes, map[int]bool{0: true}, true)
		selected := map[string]bool{"file": true}
		for _, index := range selectedIndexes {
			selected[outputModeOrder[index]] = true
		}
		modes := make([]string, 0, len(outputModeOrder))
		for _, mode := range outputModeOrder {
			if selected[mode] {
				modes = append(modes, mode)
			}
		}
		return modes
	}
	fmt.Fprintf(session.stdout, "\n%s\n", title)
	fmt.Fprintln(session.stdout, "Multi-select output modes:")
	for index, mode := range outputModeOrder {
		marker := "[ ]"
		if mode == "file" || containsInt(defaultIndexes, index) {
			marker = "[x]"
		}
		fixed := ""
		if mode == "file" {
			fixed = " - required"
		}
		fmt.Fprintf(session.stdout, "  %d) %s %s%s\n", index+1, marker, labels[mode], fixed)
	}
	answer := session.promptLine("Select optional mode numbers", indexesAnswer(defaultIndexes))
	selected := map[string]bool{"file": true}
	for _, part := range strings.Split(answer, ",") {
		number, ok := parsePositiveIndex(strings.TrimSpace(part), len(outputModeOrder))
		if ok {
			selected[outputModeOrder[number]] = true
		}
	}
	modes := make([]string, 0, len(outputModeOrder))
	for _, mode := range outputModeOrder {
		if selected[mode] {
			modes = append(modes, mode)
		}
	}
	return modes
}

func (prompt *terminalPrompt) selectRows(title string, hint string, labels []string, defaultIndexes []int, fixed map[int]bool, multi bool) []int {
	state, err := term.MakeRaw(int(prompt.stdin.Fd()))
	if err != nil {
		return defaultIndexes
	}
	defer term.Restore(int(prompt.stdin.Fd()), state)

	cursor := 0
	if len(defaultIndexes) > 0 {
		cursor = defaultIndexes[0]
	}
	selected := map[int]bool{}
	for _, index := range defaultIndexes {
		if index >= 0 && index < len(labels) {
			selected[index] = true
		}
	}
	for index := range fixed {
		if index >= 0 && index < len(labels) {
			selected[index] = true
		}
	}

	lastLines := 0
	for {
		lastLines = prompt.renderRows(lastLines, title, hint, labels, selected, fixed, cursor)
		key := prompt.readKey()
		switch key {
		case "up":
			if cursor > 0 {
				cursor--
			} else if len(labels) > 0 {
				cursor = len(labels) - 1
			}
		case "down":
			if cursor < len(labels)-1 {
				cursor++
			} else {
				cursor = 0
			}
		case "space":
			if !fixed[cursor] {
				if multi {
					selected[cursor] = !selected[cursor]
				} else {
					selected = map[int]bool{cursor: true}
				}
			}
		case "enter":
			if !multi {
				selected = map[int]bool{cursor: true}
			}
			fmt.Fprint(prompt.stdout, "\r\n")
			return selectedIndexes(selected, labels)
		case "ctrl-c":
			fmt.Fprint(prompt.stdout, "\r\n")
			return defaultIndexes
		}
	}
}

func (prompt *terminalPrompt) renderRows(lastLines int, title string, hint string, labels []string, selected map[int]bool, fixed map[int]bool, cursor int) int {
	for line := 0; line < lastLines; line++ {
		fmt.Fprint(prompt.stdout, "\x1b[1A\x1b[2K\r")
	}
	var builder strings.Builder
	builder.WriteString("\r\n")
	builder.WriteString(title)
	builder.WriteString("\r\n")
	builder.WriteString(hint)
	builder.WriteString("\r\n")
	for index, label := range labels {
		pointer := " "
		if index == cursor {
			pointer = ">"
		}
		marker := "[ ]"
		if selected[index] || fixed[index] {
			marker = "[x]"
		}
		builder.WriteString(fmt.Sprintf("%s %d) %s %s\r\n", pointer, index+1, marker, label))
	}
	text := builder.String()
	fmt.Fprint(prompt.stdout, text)
	return strings.Count(text, "\n")
}

func (prompt *terminalPrompt) readKey() string {
	buffer := make([]byte, 3)
	count, err := prompt.stdin.Read(buffer[:1])
	if err != nil || count == 0 {
		return "enter"
	}
	switch buffer[0] {
	case 3:
		return "ctrl-c"
	case ' ':
		return "space"
	case '\r', '\n':
		return "enter"
	case 0x1b:
		if count, err := prompt.stdin.Read(buffer[1:3]); err == nil && count == 2 {
			switch string(buffer[:3]) {
			case "\x1b[A":
				return "up"
			case "\x1b[B":
				return "down"
			}
		}
	}
	return ""
}

func selectedIndexes(selected map[int]bool, labels []string) []int {
	indexes := make([]int, 0, len(selected))
	for index := range labels {
		if selected[index] {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func (session *promptSession) promptLine(question string, defaultValue string) string {
	suffix := ""
	if defaultValue != "" {
		suffix = fmt.Sprintf(" [%s]", defaultValue)
	}
	fmt.Fprintf(session.stdout, "%s%s: ", question, suffix)
	answer := ""
	if session.scanner.Scan() {
		answer = strings.TrimSpace(session.scanner.Text())
	}
	fmt.Fprintln(session.stdout, answer)
	if answer == "" {
		return defaultValue
	}
	return answer
}

func (session *promptSession) promptSecret(question string, currentValue string) string {
	defaultValue := ""
	if currentValue != "" {
		defaultValue = "configured"
	}
	answer := session.promptLine(question, defaultValue)
	if answer == "configured" {
		return currentValue
	}
	return answer
}

func targetOptions() []Target {
	catalog := targetCatalog()
	keys := []string{"opencode", "claude-code", "codex", "cursor", "github-copilot"}
	options := make([]Target, 0, len(keys))
	for _, key := range keys {
		options = append(options, catalog[key])
	}
	return options
}

func targetIndexes(options []Target, keys []string) []int {
	selected := map[string]bool{}
	for _, key := range keys {
		selected[key] = true
	}
	var indexes []int
	for index, option := range options {
		if selected[option.Key] {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func indexesAnswer(indexes []int) string {
	parts := make([]string, 0, len(indexes))
	for _, index := range indexes {
		parts = append(parts, fmt.Sprintf("%d", index+1))
	}
	return strings.Join(parts, ",")
}

func targetsByAnswer(options []Target, answer string) []Target {
	seen := map[int]bool{}
	var selected []Target
	for _, part := range strings.Split(answer, ",") {
		number, ok := parsePositiveIndex(strings.TrimSpace(part), len(options))
		if ok && !seen[number] {
			selected = append(selected, options[number])
			seen[number] = true
		}
	}
	return selected
}

func parsePositiveIndex(value string, length int) (int, bool) {
	if value == "" {
		return 0, false
	}
	var number int
	if _, err := fmt.Sscanf(value, "%d", &number); err != nil {
		return 0, false
	}
	if number < 1 || number > length {
		return 0, false
	}
	return number - 1, true
}

func containsInt(values []int, needle int) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

type GeneratedPaths struct {
	SummaryPath      string
	MetadataPath     string
	ConfigPath       string
	SlackPayloadPath string
	SkillPaths       []string
}

func Generate(resolved ResolvedConfig, stdout io.Writer) (GeneratedPaths, error) {
	day := time.Now()
	discoverySource, ok := sources.Find(resolved.Source.Key)
	if !ok {
		discoverySource = sources.Source{Key: resolved.Source.Key, Label: resolved.Source.Label}
	}
	records := sources.Discover(discoverySource, sources.DiscoverOptions{Today: day})

	summaryPath := filepath.Join(resolved.OutputDir, summary.FileName(day))
	metadataPath := filepath.Join(resolved.OutputDir, summary.MetadataFileName)
	slackPayloadPath := ""
	if includesMode(resolved.OutputModes, "slack-webhook") {
		slackPayloadPath = filepath.Join(resolved.OutputDir, "ai-bricklaying-slack-payload.json")
	}
	summaryConfig := summary.Config{
		ConfigPath:             resolved.ConfigPath,
		GmailRecipient:         resolved.GmailRecipient,
		GmailSubject:           resolved.GmailSubject,
		Language:               resolved.Language,
		CLIVersion:             Version(),
		OutputModes:            resolved.OutputModes,
		OutputDir:              resolved.OutputDir,
		SkillName:              resolved.SkillName,
		SlackWebhookConfigured: resolved.SlackWebhookURL != "",
		SlackPayloadPath:       slackPayloadPath,
		Source:                 summary.Source{Key: resolved.Source.Key, Label: resolved.Source.Label},
		Targets:                summaryTargets(resolved),
		TargetModel:            resolved.TargetModel,
	}
	markdown := summary.BuildMarkdown(summaryConfig, records, day)
	if err := safeio.WriteFile(summaryPath, []byte(markdown), safeio.WriteOptions{}); err != nil {
		return GeneratedPaths{}, err
	}
	if slackPayloadPath != "" {
		payload := slack.BuildPayload(markdown)
		payloadJSON, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return GeneratedPaths{}, err
		}
		payloadJSON = []byte(safeio.RedactString(string(payloadJSON)))
		if err := safeio.WriteFile(slackPayloadPath, append(payloadJSON, '\n'), safeio.WriteOptions{}); err != nil {
			return GeneratedPaths{}, err
		}
	}

	metadata := summary.BuildMetadata(summaryConfig, summaryPath, len(records))
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return GeneratedPaths{}, err
	}
	metadataJSON = []byte(safeio.RedactString(string(metadataJSON)))
	if err := safeio.WriteFile(metadataPath, append(metadataJSON, '\n'), safeio.WriteOptions{}); err != nil {
		return GeneratedPaths{}, err
	}

	if err := writeResolvedConfig(resolved); err != nil {
		return GeneratedPaths{}, err
	}

	skillConfig := skill.Config{
		ConfigPath:       resolved.ConfigPath,
		Language:         resolved.Language,
		OutputDir:        resolved.OutputDir,
		OutputModes:      resolved.OutputModes,
		SkillName:        resolved.SkillName,
		MetadataPath:     metadataPath,
		SlackPayloadPath: slackPayloadPath,
		Sources:          []skill.Source{{Key: resolved.Source.Key, Label: resolved.Source.Label}},
	}
	skillPaths, err := skill.Install(skillConfig, skillTargets(resolved))
	if err != nil {
		return GeneratedPaths{}, err
	}

	paths := GeneratedPaths{
		SummaryPath:      summaryPath,
		MetadataPath:     metadataPath,
		ConfigPath:       resolved.ConfigPath,
		SlackPayloadPath: slackPayloadPath,
		SkillPaths:       skillPaths,
	}
	printCompletion(stdout, resolved, paths)
	return paths, nil
}

func writeResolvedConfig(resolved ResolvedConfig) error {
	stored := config.StoredConfig{
		Delivery: config.DeliveryConfig{
			GmailRecipient:  resolved.GmailRecipient,
			GmailSubject:    resolved.GmailSubject,
			SlackWebhookURL: resolved.SlackWebhookURL,
		},
		Defaults: config.DefaultsConfig{
			TargetAgents: targetKeys(resolved.TargetAgents),
			Source:       resolved.Source.Key,
			TargetModel:  resolved.TargetModel,
			Language:     resolved.Language,
			OutputModes:  append([]string(nil), resolved.OutputModes...),
			SkillName:    resolved.SkillName,
			SkillDir:     sharedSkillDir(resolved.TargetAgents),
			OutputDir:    resolved.OutputDir,
			CLIVersion:   Version(),
		},
	}
	contents, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	return safeio.WriteConfigFile(resolved.ConfigPath, append(contents, '\n'))
}

func printCompletion(stdout io.Writer, resolved ResolvedConfig, paths GeneratedPaths) {
	fmt.Fprintln(stdout, "AI Bricklaying files generated")
	fmt.Fprintf(stdout, "  Summary:  %s\n", safeio.SanitizeControl(paths.SummaryPath))
	fmt.Fprintf(stdout, "  Metadata: %s\n", safeio.SanitizeControl(paths.MetadataPath))
	fmt.Fprintf(stdout, "  Config:   %s\n", safeio.SanitizeControl(paths.ConfigPath))
	if paths.SlackPayloadPath != "" {
		fmt.Fprintf(stdout, "  Slack:    %s\n", safeio.SanitizeControl(paths.SlackPayloadPath))
	}
	for _, skillPath := range paths.SkillPaths {
		fmt.Fprintf(stdout, "  Skill:    %s\n", safeio.SanitizeControl(skillPath))
	}
	if hasTarget(resolved.TargetAgents, "opencode") {
		fmt.Fprintln(stdout, "Hint: OpenCode loads skills when a session starts. Restart OpenCode or open a new session if the skill is not visible yet.")
	}
	if includesMode(resolved.OutputModes, "gmail-mcp") {
		fmt.Fprintln(stdout, "Gmail delivery selected: Gmail MCP handoff details were prepared for the saved markdown content.")
	}
	if includesMode(resolved.OutputModes, "slack-webhook") {
		fmt.Fprintln(stdout, "Slack webhook delivery selected: Slack payload JSON was prepared; no webhook request was sent.")
	}
	fmt.Fprintf(stdout, "Use the generated skill: /%s\n", resolved.SkillName)
	fmt.Fprintln(stdout, "To refresh later: npm install -g ai-bricklaying@latest && ai-bricklaying")
}

func summaryTargets(resolved ResolvedConfig) []summary.Target {
	result := make([]summary.Target, 0, len(resolved.TargetAgents))
	for _, target := range resolved.TargetAgents {
		result = append(result, summary.Target{Key: target.Key, Label: target.Label, SkillDir: target.DefaultSkillDir})
	}
	return result
}

func skillTargets(resolved ResolvedConfig) []skill.Target {
	result := make([]skill.Target, 0, len(resolved.TargetAgents))
	for _, target := range resolved.TargetAgents {
		result = append(result, skill.Target{Key: target.Key, Label: target.Label, SkillDir: target.DefaultSkillDir})
	}
	return result
}

func sharedSkillDir(targets []Target) string {
	if len(targets) == 0 {
		return ""
	}
	first := targets[0].DefaultSkillDir
	for _, target := range targets[1:] {
		if target.DefaultSkillDir != first {
			return ""
		}
	}
	return first
}

func hasTarget(targets []Target, key string) bool {
	for _, target := range targets {
		if target.Key == key {
			return true
		}
	}
	return false
}

func includesMode(modes []string, mode string) bool {
	for _, value := range modes {
		if value == mode {
			return true
		}
	}
	return false
}

func targetsFromArgs(value string, model string) ([]Target, error) {
	keys := csv(value)
	if len(keys) == 0 {
		keys = []string{defaultTarget}
	}
	var unknown []string
	targetsByKey := targetCatalog()
	targets := make([]Target, 0, len(keys))
	for _, key := range keys {
		target, ok := targetsByKey[key]
		if !ok {
			unknown = append(unknown, key)
			continue
		}
		target.ModelHint = model
		targets = append(targets, target)
	}
	if len(unknown) > 0 {
		return nil, cliError{message: fmt.Sprintf("Unknown target agent(s): %s", strings.Join(unknown, ", "))}
	}
	return targets, nil
}

func sourceFromArgs(value string, targets []Target) (Source, error) {
	allowed := sourceOptionsForTargets(targets)
	allowedKeys := make([]string, 0, len(allowed))
	allowedSet := map[string]bool{}
	for _, source := range allowed {
		allowedKeys = append(allowedKeys, source.Key)
		allowedSet[source.Key] = true
	}

	keys := csv(value)
	if len(keys) == 0 {
		return allowed[0], nil
	}
	if len(keys) > 1 {
		return Source{}, cliError{message: "--sources accepts exactly one summary source"}
	}
	sourcesByKey := sourceCatalog()
	if _, ok := sourcesByKey[keys[0]]; !ok {
		return Source{}, cliError{message: fmt.Sprintf("Unknown source(s): %s", keys[0])}
	}
	if !allowedSet[keys[0]] {
		return Source{}, cliError{message: fmt.Sprintf("--sources must be one of the selected target agents: %s", strings.Join(allowedKeys, ", "))}
	}
	return sourcesByKey[keys[0]], nil
}

func sourceOptionsForTargets(targets []Target) []Source {
	sourcesByKey := sourceCatalog()
	seen := map[string]bool{}
	var sources []Source
	for _, target := range targets {
		source, ok := sourcesByKey[target.Key]
		if ok && !seen[source.Key] {
			sources = append(sources, source)
			seen[source.Key] = true
		}
	}
	return sources
}

func outputModesFromArgs(args ParsedArgs) ([]string, error) {
	modes, err := outputModesFromValue(args.OutputModes)
	if err != nil {
		return nil, err
	}
	modeSet := map[string]bool{}
	for _, mode := range modes {
		modeSet[mode] = true
	}
	if args.NonInteractive {
		if modeSet["gmail-mcp"] && (args.GmailRecipient == "" || args.GmailSubject == "") {
			return nil, cliError{message: "gmail-mcp requires --gmail-recipient and --gmail-subject"}
		}
		if modeSet["slack-webhook"] && args.SlackWebhookURL == "" {
			return nil, cliError{message: "slack-webhook requires --slack-webhook-url"}
		}
	}
	return modes, nil
}

func outputModesFromValue(value string) ([]string, error) {
	modeSet := map[string]bool{"file": true}
	for _, mode := range csv(value) {
		modeSet[outputModeKey(mode)] = true
	}
	var unknown []string
	for mode := range modeSet {
		if !isKnownOutputMode(mode) {
			unknown = append(unknown, mode)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, cliError{message: fmt.Sprintf("Unknown output mode(s): %s", strings.Join(unknown, ", "))}
	}
	var modes []string
	for _, mode := range outputModeOrder {
		if modeSet[mode] {
			modes = append(modes, mode)
		}
	}
	return modes, nil
}

func outputModeKey(value string) string {
	switch value {
	case "gmail":
		return "gmail-mcp"
	case "slack":
		return "slack-webhook"
	default:
		return value
	}
}

func validateSkillName(value string) error {
	if !skillNameRegexp.MatchString(value) || strings.Contains(value, "..") {
		return cliError{message: "--skill-name must be a path-safe slug using lowercase letters, numbers, dots, underscores, or hyphens"}
	}
	return nil
}

func isKnownOutputMode(value string) bool {
	for _, mode := range outputModeOrder {
		if value == mode {
			return true
		}
	}
	return false
}

func csv(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func targetKeys(targets []Target) []string {
	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		keys = append(keys, target.Key)
	}
	return keys
}

func targetCatalog() map[string]Target {
	home := homeDir()
	return map[string]Target{
		"opencode":       {Key: "opencode", Label: "OpenCode", DefaultSkillDir: filepath.Join(home, ".config/opencode/skills"), ModelHint: "configured OpenCode model"},
		"claude-code":    {Key: "claude-code", Label: "Claude Code", DefaultSkillDir: filepath.Join(home, ".claude/skills"), ModelHint: "configured Claude model"},
		"codex":          {Key: "codex", Label: "Codex", DefaultSkillDir: filepath.Join(home, ".codex/skills"), ModelHint: "configured Codex model"},
		"cursor":         {Key: "cursor", Label: "Cursor", DefaultSkillDir: filepath.Join(home, ".cursor/skills"), ModelHint: "configured Cursor model"},
		"github-copilot": {Key: "github-copilot", Label: "GitHub Copilot", DefaultSkillDir: filepath.Join(home, ".github-copilot/skills"), ModelHint: "configured Copilot model"},
	}
}

func sourceCatalog() map[string]Source {
	return map[string]Source{
		"opencode":       {Key: "opencode", Label: "OpenCode"},
		"claude-code":    {Key: "claude-code", Label: "Claude Code"},
		"codex":          {Key: "codex", Label: "Codex"},
		"cursor":         {Key: "cursor", Label: "Cursor"},
		"github-copilot": {Key: "github-copilot", Label: "GitHub Copilot"},
	}
}

func absolutePath(value string) string {
	if value == "" {
		return ""
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return filepath.Clean(value)
	}
	return absolute
}

func defaultOutputDir() string {
	return filepath.Join(homeDir(), "ai-bricklaying")
}

func defaultConfigDir() string {
	return filepath.Join(homeDir(), ".config/ai-bricklaying")
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}
