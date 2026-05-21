package sources

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"ai-bricklaying/internal/config"
	"ai-bricklaying/internal/safeio"
)

const (
	DefaultRecordLimit = 12
	DefaultMaxChars    = 20_000
)

var (
	textExtensions = map[string]bool{
		".json":  true,
		".jsonl": true,
		".md":    true,
		".txt":   true,
		".log":   true,
	}
	jsonTextKeys = []string{"text", "content", "message", "prompt", "response", "summary"}
)

// Source describes a supported local AI session source.
type Source struct {
	Key          string
	Label        string
	DefaultPaths []string
	EnvVar       string
}

// Record is a normalized, redacted signal extracted from a local session artifact.
type Record struct {
	SourceKey   string
	SourceLabel string
	Path        string
	Text        string
	ModifiedAt  time.Time
}

// DiscoverOptions controls local session discovery for tests and later CLI integration.
type DiscoverOptions struct {
	Today      time.Time
	Limit      int
	MaxChars   int
	EnvLookup  func(string) string
	PathLookup []string
}

type candidate struct {
	path    string
	modTime time.Time
}

// Catalog returns all supported session sources in stable CLI order.
func Catalog() []Source {
	home := homeDir()
	return []Source{
		{Key: "opencode", Label: "OpenCode", DefaultPaths: []string{filepath.Join(home, ".local/share/opencode")}, EnvVar: "AI_BRICKLAYING_OPENCODE_DIRS"},
		{Key: "claude-code", Label: "Claude Code", DefaultPaths: []string{filepath.Join(home, ".claude/projects")}, EnvVar: "AI_BRICKLAYING_CLAUDE_DIRS"},
		{Key: "codex", Label: "Codex", DefaultPaths: []string{filepath.Join(home, ".codex/sessions"), filepath.Join(home, ".codex")}, EnvVar: "AI_BRICKLAYING_CODEX_DIRS"},
		{Key: "cursor", Label: "Cursor", DefaultPaths: []string{filepath.Join(home, "Library/Application Support/Cursor/User/workspaceStorage")}, EnvVar: "AI_BRICKLAYING_CURSOR_DIRS"},
		{Key: "github-copilot", Label: "GitHub Copilot", DefaultPaths: []string{filepath.Join(home, "Library/Application Support/Code/User/workspaceStorage")}, EnvVar: "AI_BRICKLAYING_COPILOT_DIRS"},
	}
}

// Find returns a supported source by key.
func Find(key string) (Source, bool) {
	for _, source := range Catalog() {
		if source.Key == key {
			return source, true
		}
	}
	return Source{}, false
}

// DiscoverToday collects today's redacted records for source using default limits.
func DiscoverToday(source Source) []Record {
	return Discover(source, DiscoverOptions{})
}

// Discover collects redacted records for source. Missing, unreadable, unsupported, and symlinked paths are skipped.
func Discover(source Source, options DiscoverOptions) []Record {
	today := options.Today
	if today.IsZero() {
		today = time.Now()
	}
	limit := options.Limit
	if limit <= 0 {
		limit = DefaultRecordLimit
	}
	maxChars := options.MaxChars
	if maxChars <= 0 {
		maxChars = DefaultMaxChars
	}

	candidates := collectCandidates(sourcePaths(source, options))
	sort.SliceStable(candidates, func(left, right int) bool {
		return candidates[left].modTime.After(candidates[right].modTime)
	})

	records := make([]Record, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		if len(records) >= limit {
			break
		}
		if !sameLocalDate(candidate.modTime, today) {
			continue
		}
		text := readSessionText(candidate.path, maxChars)
		if text == "" {
			continue
		}
		records = append(records, Record{
			SourceKey:   source.Key,
			SourceLabel: source.Label,
			Path:        candidate.path,
			Text:        safeio.RedactString(text),
			ModifiedAt:  candidate.modTime,
		})
	}
	return records
}

func sourcePaths(source Source, options DiscoverOptions) []string {
	if len(options.PathLookup) > 0 {
		return cleanPaths(options.PathLookup)
	}
	lookup := options.EnvLookup
	if lookup == nil {
		lookup = os.Getenv
	}
	if configured := lookup(source.EnvVar); configured != "" {
		return cleanPaths(strings.Split(configured, string(os.PathListSeparator)))
	}
	return cleanPaths(source.DefaultPaths)
}

func cleanPaths(paths []string) []string {
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, config.ExpandHome(trimmed))
	}
	return cleaned
}

func collectCandidates(roots []string) []candidate {
	var candidates []candidate
	for _, root := range roots {
		info, err := os.Lstat(root)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.Mode().IsRegular() {
			if isSupported(root) {
				candidates = append(candidates, candidate{path: root, modTime: info.ModTime()})
			}
			continue
		}
		if !info.IsDir() {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() || !isSupported(path) {
				return nil
			}
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			candidates = append(candidates, candidate{path: path, modTime: info.ModTime()})
			return nil
		})
	}
	return candidates
}

func isSupported(path string) bool {
	return textExtensions[strings.ToLower(filepath.Ext(path))]
}

func sameLocalDate(left time.Time, right time.Time) bool {
	left = left.Local()
	right = right.Local()
	leftYear, leftMonth, leftDay := left.Date()
	rightYear, rightMonth, rightDay := right.Date()
	return leftYear == rightYear && leftMonth == rightMonth && leftDay == rightDay
}

func readSessionText(path string, maxChars int) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, int64(maxChars*utf8.UTFMax)+1))
	if err != nil {
		return ""
	}
	raw := truncateRunes(string(contents), maxChars)
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".jsonl":
		return jsonlToText(raw)
	case ".json":
		return jsonToText(raw)
	default:
		return strings.TrimSpace(raw)
	}
}

func jsonlToText(raw string) string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), DefaultMaxChars*utf8.UTFMax)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			lines = append(lines, line)
			continue
		}
		lines = append(lines, extractTextValues(value)...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func jsonToText(raw string) string {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return strings.TrimSpace(raw)
	}
	return strings.TrimSpace(strings.Join(extractTextValues(value), "\n"))
}

func extractTextValues(value any) []string {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		return []string{text}
	case []any:
		var result []string
		for _, item := range typed {
			result = append(result, extractTextValues(item)...)
		}
		return result
	case map[string]any:
		var result []string
		for _, key := range jsonTextKeys {
			if item, ok := typed[key]; ok {
				result = append(result, extractTextValues(item)...)
			}
		}
		return result
	default:
		return nil
	}
}

func truncateRunes(value string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	builder := strings.Builder{}
	builder.Grow(maxChars)
	count := 0
	for _, char := range value {
		if count >= maxChars {
			break
		}
		builder.WriteRune(char)
		count++
	}
	return builder.String()
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}
