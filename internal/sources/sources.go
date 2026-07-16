package sources

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"ai-bricklaying/internal/config"
	"ai-bricklaying/internal/safeio"
)

const (
	DefaultRecordLimit        = 12
	DefaultMaxChars           = 20_000
	DefaultCandidateFileLimit = 5_000
	DefaultScanFileBytes      = 16 << 20
	defaultJSONLPrefixBytes   = 1 << 20
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
	// TimestampBasis is event_timestamp for strict adapters and may be
	// file_modified_at for a documented best-effort fallback. Legacy summary
	// discovery leaves it empty.
	TimestampBasis string
}

// Coverage describes whether a source produced usable records for a requested day.
// It intentionally contains no local paths so it is safe to expose to an agent.
type Coverage struct {
	SourceKey      string `json:"source_key"`
	SourceLabel    string `json:"source_label"`
	Status         string `json:"status"`
	CandidateFiles int    `json:"candidate_files"`
	UsedRecords    int    `json:"used_records"`
	Reason         string `json:"reason,omitempty"`
}

// Report combines redacted records with path-free source coverage.
type Report struct {
	Records  []Record
	Coverage Coverage
}

// DiscoverOptions controls local session discovery for tests and later CLI integration.
type DiscoverOptions struct {
	Today      time.Time
	Limit      int
	MaxChars   int
	FileLimit  int
	EnvLookup  func(string) string
	PathLookup []string
}

type candidate struct {
	path    string
	modTime time.Time
}

type sessionReadResult struct {
	text       string
	parseError bool
	truncated  bool
	unreadable bool
}

// Catalog returns all supported session sources in stable CLI order.
func Catalog() []Source {
	return catalogForPlatform(homeDir(), runtime.GOOS)
}

func catalogForPlatform(home string, goos string) []Source {
	cursorPaths, copilotPaths := editorSourcePaths(home, goos)
	opencodePaths := []string{filepath.Join(home, ".local/share/opencode")}
	if goos == "windows" {
		opencodePaths = []string{filepath.Join(home, "AppData/Local/opencode")}
	}
	return []Source{
		{Key: "opencode", Label: "OpenCode", DefaultPaths: opencodePaths, EnvVar: "AI_BRICKLAYING_OPENCODE_DIRS"},
		{Key: "claude-code", Label: "Claude Code", DefaultPaths: []string{filepath.Join(home, ".claude/projects")}, EnvVar: "AI_BRICKLAYING_CLAUDE_DIRS"},
		// Discover and DiscoverReport are the legacy single-source summary
		// contract. Keep the broad historical Codex root here; strict daily
		// worklog discovery substitutes source-specific roots before walking.
		{Key: "codex", Label: "Codex", DefaultPaths: []string{filepath.Join(home, ".codex/sessions"), filepath.Join(home, ".codex")}, EnvVar: "AI_BRICKLAYING_CODEX_DIRS"},
		{Key: "cursor", Label: "Cursor", DefaultPaths: cursorPaths, EnvVar: "AI_BRICKLAYING_CURSOR_DIRS"},
		{Key: "github-copilot", Label: "GitHub Copilot", DefaultPaths: copilotPaths, EnvVar: "AI_BRICKLAYING_COPILOT_DIRS"},
	}
}

func editorSourcePaths(home string, goos string) ([]string, []string) {
	switch goos {
	case "darwin":
		return []string{filepath.Join(home, "Library/Application Support/Cursor/User/workspaceStorage")},
			[]string{filepath.Join(home, "Library/Application Support/Code/User/workspaceStorage")}
	case "windows":
		return []string{filepath.Join(home, "AppData/Roaming/Cursor/User/workspaceStorage")},
			[]string{filepath.Join(home, "AppData/Roaming/Code/User/workspaceStorage")}
	default:
		return []string{filepath.Join(home, ".config/Cursor/User/workspaceStorage")},
			[]string{filepath.Join(home, ".config/Code/User/workspaceStorage")}
	}
}

func worklogDefaultPaths(source Source) []string {
	return worklogDefaultPathsAtHome(source, homeDir())
}

// worklogDefaultPathsAtHome isolates strict daily evidence roots from the
// broader legacy summary catalog. In particular, it never exposes the whole
// Codex config tree or Cursor workspaceStorage to source-specific adapters.
func worklogDefaultPathsAtHome(source Source, home string) []string {
	switch source.Key {
	case "codex":
		return []string{
			filepath.Join(home, ".codex/sessions"),
			filepath.Join(home, ".codex/archived_sessions"),
		}
	case "cursor":
		return []string{filepath.Join(home, ".cursor/projects")}
	default:
		return append([]string(nil), source.DefaultPaths...)
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
	return DiscoverReport(source, options).Records
}

// DiscoverReport collects records and reports missing/no-activity/parse-error/truncated coverage
// without exposing discovery roots or record paths.
func DiscoverReport(source Source, options DiscoverOptions) Report {
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

	fileLimit := options.FileLimit
	if fileLimit <= 0 {
		fileLimit = DefaultCandidateFileLimit
	}

	paths := sourcePaths(source, options)
	rootsFound, unreadableRoot := inspectRoots(paths)
	candidates, candidateScanTruncated, candidateTraversalUnreadable := collectCandidates(paths, fileLimit, today)
	sort.SliceStable(candidates, func(left, right int) bool {
		return candidates[left].modTime.After(candidates[right].modTime)
	})

	records := make([]Record, 0, min(limit, len(candidates)))
	todayCandidates := 0
	truncated := candidateScanTruncated
	parseError := false
	unreadableCandidate := candidateTraversalUnreadable
	for _, candidate := range candidates {
		if !sameLocalDate(candidate.modTime, today) {
			continue
		}
		todayCandidates++
		if len(records) >= limit {
			truncated = true
			continue
		}
		readResult := readSessionText(candidate.path, source.Key)
		parseError = parseError || readResult.parseError
		truncated = truncated || readResult.truncated
		unreadableCandidate = unreadableCandidate || readResult.unreadable
		text := strings.TrimSpace(safeio.RedactString(readResult.text))
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) > maxChars {
			text = truncateRunes(text, maxChars)
			truncated = true
		}
		records = append(records, Record{
			SourceKey:   source.Key,
			SourceLabel: source.Label,
			Path:        candidate.path,
			Text:        text,
			ModifiedAt:  candidate.modTime,
		})
	}

	status := "no_activity"
	reason := ""
	switch {
	case !rootsFound && unreadableRoot:
		status = "unreadable"
		reason = "configured source root could not be read"
	case !rootsFound:
		status = "not_found"
		reason = "configured source root was not found"
	case parseError:
		status = "parse_error"
		reason = "one or more JSON session artifacts could not be parsed"
	case unreadableRoot || unreadableCandidate:
		status = "unreadable"
		reason = "one or more configured source paths could not be read"
	case len(records) > 0 && truncated:
		status = "truncated"
		reason = "source exceeded the per-run record or excerpt limit"
	case len(records) > 0:
		status = "complete"
	case truncated:
		status = "truncated"
		reason = "source exceeded the per-run file or excerpt limit"
	default:
		status = "no_activity"
		reason = "no readable activity was found for the requested date"
	}

	return Report{
		Records: records,
		Coverage: Coverage{
			SourceKey:      source.Key,
			SourceLabel:    source.Label,
			Status:         status,
			CandidateFiles: todayCandidates,
			UsedRecords:    len(records),
			Reason:         reason,
		},
	}
}

func inspectRoots(paths []string) (found bool, unreadable bool) {
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			if !os.IsNotExist(err) {
				unreadable = true
			}
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			found = true
		}
	}
	return found, unreadable
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
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		expanded := filepath.Clean(config.ExpandHome(trimmed))
		identity := candidateIdentity(expanded)
		if _, exists := seen[identity]; exists {
			continue
		}
		seen[identity] = struct{}{}
		cleaned = append(cleaned, expanded)
	}
	return cleaned
}

func collectCandidates(roots []string, fileLimit int, today time.Time) ([]candidate, bool, bool) {
	candidates := make([]candidate, 0, min(fileLimit, DefaultRecordLimit))
	seen := make(map[string]struct{})
	capped := false
	unreadable := false
	addCandidate := func(path string, info fs.FileInfo) bool {
		if !sameLocalDate(info.ModTime(), today) {
			return true
		}
		identity := candidateIdentity(path)
		if _, exists := seen[identity]; exists {
			return true
		}
		if len(candidates) >= fileLimit {
			capped = true
			return false
		}
		seen[identity] = struct{}{}
		candidates = append(candidates, candidate{path: filepath.Clean(path), modTime: info.ModTime()})
		return true
	}

	var walkNewest func(string) bool
	walkNewest = func(dir string) bool {
		entries, err := os.ReadDir(dir)
		if err != nil {
			unreadable = true
			return true
		}
		type rankedEntry struct {
			entry os.DirEntry
			info  fs.FileInfo
		}
		ranked := make([]rankedEntry, 0, len(entries))
		for _, entry := range entries {
			info, infoErr := entry.Info()
			if infoErr != nil {
				unreadable = true
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			ranked = append(ranked, rankedEntry{entry: entry, info: info})
		}
		sort.SliceStable(ranked, func(left, right int) bool {
			if ranked[left].info.ModTime().Equal(ranked[right].info.ModTime()) {
				return ranked[left].entry.Name() > ranked[right].entry.Name()
			}
			return ranked[left].info.ModTime().After(ranked[right].info.ModTime())
		})
		for _, item := range ranked {
			path := filepath.Join(dir, item.entry.Name())
			if item.info.IsDir() {
				if !walkNewest(path) {
					return false
				}
				continue
			}
			if !item.info.Mode().IsRegular() || !isSupported(path) {
				continue
			}
			if !addCandidate(path, item.info) {
				return false
			}
		}
		return true
	}

	for _, root := range roots {
		if capped {
			break
		}
		info, err := os.Lstat(root)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.Mode().IsRegular() {
			if isSupported(root) {
				addCandidate(root, info)
			}
			continue
		}
		if !info.IsDir() {
			continue
		}
		walkNewest(root)
	}
	return candidates, capped, unreadable
}

func candidateIdentity(path string) string {
	cleaned := filepath.Clean(path)
	absolute, err := filepath.Abs(cleaned)
	if err == nil {
		return absolute
	}
	return cleaned
}

func isSupported(path string) bool {
	return textExtensions[strings.ToLower(filepath.Ext(path))]
}

func sameLocalDate(left time.Time, right time.Time) bool {
	location := right.Location()
	left = left.In(location)
	right = right.In(location)
	leftYear, leftMonth, leftDay := left.Date()
	rightYear, rightMonth, rightDay := right.Date()
	return leftYear == rightYear && leftMonth == rightMonth && leftDay == rightDay
}

func readSessionText(path string, sourceKey string) sessionReadResult {
	file, err := os.Open(path)
	if err != nil {
		return sessionReadResult{unreadable: true}
	}
	defer file.Close()
	if strings.EqualFold(filepath.Ext(path), ".jsonl") {
		return readJSONLTail(file, sourceKey)
	}

	contents, err := io.ReadAll(io.LimitReader(file, DefaultScanFileBytes+1))
	if err != nil {
		return sessionReadResult{unreadable: true}
	}
	inputTruncated := len(contents) > DefaultScanFileBytes
	if inputTruncated {
		contents = contents[:DefaultScanFileBytes]
	}
	raw := string(contents)
	if strings.TrimSpace(raw) == "" {
		return sessionReadResult{truncated: inputTruncated}
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		if inputTruncated {
			return sessionReadResult{truncated: true}
		}
		text, err := jsonToText(raw)
		return sessionReadResult{text: text, parseError: err != nil}
	default:
		return sessionReadResult{text: strings.TrimSpace(raw), truncated: inputTruncated}
	}
}

func readJSONLTail(file *os.File, sourceKey string) sessionReadResult {
	info, err := file.Stat()
	if err != nil {
		return sessionReadResult{unreadable: true}
	}
	start := int64(0)
	truncated := info.Size() > DefaultScanFileBytes
	tailBytes := int64(DefaultScanFileBytes)
	if truncated {
		tailBytes -= defaultJSONLPrefixBytes
		start = info.Size() - tailBytes
		// Read one preceding byte so we can distinguish a record boundary from
		// the middle of a line without ever exposing a partial JSON record.
		if start > 0 {
			start--
		}
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return sessionReadResult{unreadable: true}
	}
	contents, err := io.ReadAll(io.LimitReader(file, tailBytes+1))
	if err != nil {
		return sessionReadResult{unreadable: true}
	}
	if truncated && len(contents) > 0 {
		if contents[0] == '\n' {
			contents = contents[1:]
		} else if boundary := bytes.IndexByte(contents, '\n'); boundary >= 0 {
			contents = contents[boundary+1:]
		} else {
			contents = nil
		}
	}
	text, parseError := jsonlToText(string(contents), sourceKey)
	if truncated && strings.TrimSpace(text) == "" {
		prefixText, prefixParseError, unreadable := readJSONLPrefix(file, sourceKey)
		if unreadable {
			return sessionReadResult{parseError: parseError, truncated: true, unreadable: true}
		}
		return sessionReadResult{
			text:       prefixText,
			parseError: parseError || prefixParseError,
			truncated:  true,
		}
	}
	return sessionReadResult{text: text, parseError: parseError, truncated: truncated}
}

func readJSONLPrefix(file *os.File, sourceKey string) (string, bool, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", false, true
	}
	contents, err := io.ReadAll(io.LimitReader(file, defaultJSONLPrefixBytes))
	if err != nil {
		return "", false, true
	}
	boundary := bytes.LastIndexByte(contents, '\n')
	if boundary < 0 {
		return "", false, false
	}
	text, parseError := jsonlToText(string(contents[:boundary]), sourceKey)
	return text, parseError, false
}

func jsonlToText(raw string, sourceKey string) (string, bool) {
	var lines []string
	parseError := false
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), DefaultScanFileBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		value, err := decodeJSONValue(line)
		if err != nil {
			parseError = true
			continue
		}
		if sourceKey == "codex" {
			lines = append(lines, extractCodexTextValues(value)...)
			continue
		}
		lines = append(lines, extractTextValues(value)...)
	}
	if scanner.Err() != nil {
		parseError = true
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), parseError
}

func extractCodexTextValues(value any) []string {
	event, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	eventType, _ := event["type"].(string)
	payload, ok := event["payload"].(map[string]any)
	if !ok {
		return nil
	}

	switch eventType {
	case "response_item":
		return extractCodexResponseMessage(payload)
	case "event_msg":
		return extractCodexEventMessage(payload)
	default:
		return nil
	}
}

func extractCodexResponseMessage(payload map[string]any) []string {
	if payload["type"] != "message" {
		return nil
	}
	role, _ := payload["role"].(string)
	if role != "user" && role != "assistant" {
		return nil
	}
	contents, ok := payload["content"].([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(contents))
	for _, value := range contents {
		content, ok := value.(map[string]any)
		if !ok {
			continue
		}
		contentType, _ := content["type"].(string)
		if role == "user" && contentType != "input_text" {
			continue
		}
		if role == "assistant" && contentType != "output_text" {
			continue
		}
		if text := normalizedText(content["text"]); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func extractCodexEventMessage(payload map[string]any) []string {
	payloadType, _ := payload["type"].(string)
	if payloadType != "user_message" && payloadType != "agent_message" {
		return nil
	}
	if text := normalizedText(payload["message"]); text != "" {
		return []string{text}
	}
	return nil
}

func normalizedText(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func jsonToText(raw string) (string, error) {
	value, err := decodeJSONValue(raw)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.Join(extractTextValues(value), "\n")), nil
}

func decodeJSONValue(raw string) (any, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values are not allowed")
		}
		return nil, err
	}
	return value, nil
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
