package sources

import (
	"container/heap"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"ai-bricklaying/internal/config"
	"ai-bricklaying/internal/safeio"
)

type sourceAdapter struct {
	match func(root string, path string, direct bool) bool
	parse func(path string, day time.Time) artifactResult
}

type adapterCandidateHeapEntry struct {
	candidate candidate
	identity  string
}

// adapterCandidateMinHeap keeps the least-preferred retained candidate at the
// root so a newer candidate can replace it in O(log fileLimit) time.
type adapterCandidateMinHeap []adapterCandidateHeapEntry

func (entries adapterCandidateMinHeap) Len() int { return len(entries) }

func (entries adapterCandidateMinHeap) Less(left, right int) bool {
	return adapterCandidateOlder(entries[left].candidate, entries[right].candidate)
}

func (entries adapterCandidateMinHeap) Swap(left, right int) {
	entries[left], entries[right] = entries[right], entries[left]
}

func (entries *adapterCandidateMinHeap) Push(value any) {
	*entries = append(*entries, value.(adapterCandidateHeapEntry))
}

func (entries *adapterCandidateMinHeap) Pop() any {
	old := *entries
	last := len(old) - 1
	value := old[last]
	old[last] = adapterCandidateHeapEntry{}
	*entries = old[:last]
	return value
}

type boundedAdapterCandidates struct {
	limit     int
	entries   adapterCandidateMinHeap
	retained  map[string]struct{}
	truncated bool
}

func newBoundedAdapterCandidates(limit int) *boundedAdapterCandidates {
	initialCapacity := min(limit, DefaultRecordLimit)
	return &boundedAdapterCandidates{
		limit:    limit,
		entries:  make(adapterCandidateMinHeap, 0, initialCapacity),
		retained: make(map[string]struct{}, initialCapacity),
	}
}

func (bounded *boundedAdapterCandidates) add(value candidate, identity string) {
	if _, exists := bounded.retained[identity]; exists {
		return
	}
	entry := adapterCandidateHeapEntry{candidate: value, identity: identity}
	if bounded.entries.Len() < bounded.limit {
		heap.Push(&bounded.entries, entry)
		bounded.retained[identity] = struct{}{}
		return
	}

	// An identity absent from a full retained set is either a new unique
	// candidate or a duplicate that was already evicted. In both cases more
	// than limit unique candidates have been observed, so coverage is truncated.
	bounded.truncated = true
	if !adapterCandidateNewer(value, bounded.entries[0].candidate) {
		return
	}
	displaced := heap.Pop(&bounded.entries).(adapterCandidateHeapEntry)
	delete(bounded.retained, displaced.identity)
	heap.Push(&bounded.entries, entry)
	bounded.retained[identity] = struct{}{}
}

func (bounded *boundedAdapterCandidates) sorted() []candidate {
	candidates := make([]candidate, len(bounded.entries))
	for index, entry := range bounded.entries {
		candidates[index] = entry.candidate
	}
	sort.Slice(candidates, func(left, right int) bool {
		return adapterCandidateNewer(candidates[left], candidates[right])
	})
	return candidates
}

func adapterCandidateNewer(left, right candidate) bool {
	if left.modTime.Equal(right.modTime) {
		return left.path > right.path
	}
	return left.modTime.After(right.modTime)
}

func adapterCandidateOlder(left, right candidate) bool {
	if left.modTime.Equal(right.modTime) {
		return left.path < right.path
	}
	return left.modTime.Before(right.modTime)
}

func adapterFor(sourceKey string) (sourceAdapter, bool) {
	switch sourceKey {
	case "claude-code":
		return sourceAdapter{match: matchClaudeArtifact, parse: parseClaudeArtifact}, true
	case "codex":
		return sourceAdapter{match: matchCodexArtifact, parse: parseCodexArtifact}, true
	case "cursor":
		return sourceAdapter{match: matchCursorArtifact, parse: parseCursorArtifact}, true
	case "github-copilot":
		return sourceAdapter{match: matchCopilotArtifact, parse: parseCopilotArtifact}, true
	case "opencode":
		return sourceAdapter{match: matchOpenCodeArtifact, parse: parseOpenCodeArtifact}, true
	default:
		return sourceAdapter{}, false
	}
}

// DiscoverWorklogReport uses strict source-specific adapters for daily
// evidence. Discover and DiscoverReport intentionally retain the legacy
// summary extractor contract.
func DiscoverWorklogReport(source Source, options DiscoverOptions) Report {
	day := options.Today
	if day.IsZero() {
		day = time.Now()
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

	adapter, supported := adapterFor(source.Key)
	if !supported {
		return strictReport(source, nil, "unsupported_schema", 0,
			"no source-specific worklog adapter is available")
	}

	paths := worklogSourcePaths(source, options)
	rootsFound, unreadableRoot := inspectRoots(paths)
	if !rootsFound {
		if unreadableRoot {
			return strictReport(source, nil, "unreadable", 0,
				"configured source root could not be read")
		}
		return strictReport(source, nil, "not_found", 0,
			"configured source root was not found")
	}

	candidates, candidateTruncated, traversalUnreadable := collectAdapterCandidates(paths, fileLimit, adapter.match)
	records := make([]Record, 0, min(limit, len(candidates)))
	parseError := false
	truncated := candidateTruncated
	unreadable := unreadableRoot || traversalUnreadable
	unsupportedSchema := false
	unsupportedStorage := false

	for _, candidate := range candidates {
		result := adapter.parse(candidate.path, day)
		parseError = parseError || result.ParseError
		truncated = truncated || result.Truncated
		unreadable = unreadable || result.Unreadable
		unsupportedSchema = unsupportedSchema || result.UnsupportedSchema
		unsupportedStorage = unsupportedStorage || result.UnsupportedStorage

		record, ok, recordTruncated, invalidTimestamp := recordFromEvents(source, candidate, result.Events, day, maxChars)
		truncated = truncated || recordTruncated
		unsupportedSchema = unsupportedSchema || invalidTimestamp
		if ok {
			records = append(records, record)
		}
	}

	sort.SliceStable(records, func(left, right int) bool {
		if records[left].ModifiedAt.Equal(records[right].ModifiedAt) {
			return records[left].Path > records[right].Path
		}
		return records[left].ModifiedAt.After(records[right].ModifiedAt)
	})
	if len(records) > limit {
		records = records[:limit]
		truncated = true
	}

	status := "no_activity"
	reason := "no verified activity was found for the requested date"
	switch {
	case unsupportedStorage:
		status = "unsupported_storage"
		reason = "the detected source storage cannot be read safely by this release"
	case unsupportedSchema:
		status = "unsupported_schema"
		reason = "one or more source artifacts used an unrecognized or unverifiable schema"
	case parseError:
		status = "parse_error"
		reason = "one or more source artifacts contained malformed JSON"
	case unreadable:
		status = "unreadable"
		reason = "one or more configured source artifacts could not be read"
	case len(records) > 0 && truncated:
		status = "truncated"
		reason = "source exceeded the per-run artifact, record, or excerpt limit"
	case len(records) > 0:
		status = "complete"
		reason = ""
	case truncated:
		status = "truncated"
		reason = "source exceeded the per-run artifact or read limit"
	}
	return strictReport(source, records, status, len(candidates), reason)
}

func strictReport(source Source, records []Record, status string, candidates int, reason string) Report {
	return Report{
		Records: records,
		Coverage: Coverage{
			SourceKey:      source.Key,
			SourceLabel:    source.Label,
			Status:         status,
			CandidateFiles: candidates,
			UsedRecords:    len(records),
			Reason:         reason,
		},
	}
}

func worklogSourcePaths(source Source, options DiscoverOptions) []string {
	if len(options.PathLookup) > 0 {
		return cleanPaths(options.PathLookup)
	}
	lookup := options.EnvLookup
	if lookup == nil {
		lookup = os.Getenv
	}
	if configured := strings.TrimSpace(lookup(source.EnvVar)); configured != "" {
		return cleanPaths(strings.Split(configured, string(os.PathListSeparator)))
	}
	switch source.Key {
	case "claude-code":
		if configDir := strings.TrimSpace(lookup("CLAUDE_CONFIG_DIR")); configDir != "" {
			return cleanPaths([]string{filepath.Join(config.ExpandHome(configDir), "projects")})
		}
	case "codex":
		if codexHome := strings.TrimSpace(lookup("CODEX_HOME")); codexHome != "" {
			codexHome = config.ExpandHome(codexHome)
			return cleanPaths([]string{
				filepath.Join(codexHome, "sessions"),
				filepath.Join(codexHome, "archived_sessions"),
			})
		}
	case "opencode":
		if dataHome := strings.TrimSpace(lookup("XDG_DATA_HOME")); dataHome != "" {
			return cleanPaths([]string{filepath.Join(config.ExpandHome(dataHome), "opencode")})
		}
	}
	return cleanPaths(worklogDefaultPaths(source))
}

func collectAdapterCandidates(roots []string, fileLimit int, match func(string, string, bool) bool) ([]candidate, bool, bool) {
	if fileLimit <= 0 {
		fileLimit = DefaultCandidateFileLimit
	}
	bounded := newBoundedAdapterCandidates(fileLimit)
	unreadable := false
	add := func(root string, path string, info fs.FileInfo, direct bool) {
		if !info.Mode().IsRegular() || !match(root, path, direct) {
			return
		}
		identity := candidateIdentity(path)
		bounded.add(candidate{path: filepath.Clean(path), modTime: info.ModTime()}, identity)
	}

	for _, root := range roots {
		info, err := os.Lstat(root)
		if err != nil {
			if !os.IsNotExist(err) {
				unreadable = true
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.Mode().IsRegular() {
			add(root, root, info, true)
			continue
		}
		if !info.IsDir() {
			continue
		}
		walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				unreadable = true
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if path == root {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				unreadable = true
				return nil
			}
			add(root, path, info, false)
			return nil
		})
		if walkErr != nil {
			unreadable = true
		}
	}

	return bounded.sorted(), bounded.truncated, unreadable
}

func recordFromEvents(source Source, candidate candidate, events []sourceEvent, day time.Time, maxChars int) (Record, bool, bool, bool) {
	verified := make([]sourceEvent, 0, len(events))
	invalidTimestamp := false
	for _, event := range events {
		if event.Timestamp.IsZero() {
			if strings.TrimSpace(event.Text) != "" {
				invalidTimestamp = true
			}
			continue
		}
		if !isRequestedDay(event.Timestamp, day) {
			continue
		}
		if strings.TrimSpace(event.Text) == "" {
			continue
		}
		verified = append(verified, event)
	}
	if len(verified) == 0 {
		return Record{}, false, false, invalidTimestamp
	}
	sort.SliceStable(verified, func(left, right int) bool {
		if verified[left].Timestamp.Equal(verified[right].Timestamp) {
			return verified[left].Sequence > verified[right].Sequence
		}
		return verified[left].Timestamp.After(verified[right].Timestamp)
	})

	var builder strings.Builder
	seen := make(map[string]struct{})
	used := 0
	truncated := false
	for _, event := range verified {
		text := strings.TrimSpace(safeio.RedactString(event.Text))
		if text == "" {
			continue
		}
		if _, duplicate := seen[text]; duplicate {
			continue
		}
		seen[text] = struct{}{}
		separator := 0
		if builder.Len() > 0 {
			separator = 1
		}
		remaining := maxChars - used - separator
		if remaining <= 0 {
			truncated = true
			break
		}
		if separator == 1 {
			builder.WriteByte('\n')
			used++
		}
		if utf8.RuneCountInString(text) > remaining {
			builder.WriteString(truncateRunes(text, remaining))
			used += remaining
			truncated = true
			break
		}
		builder.WriteString(text)
		used += utf8.RuneCountInString(text)
	}
	if strings.TrimSpace(builder.String()) == "" {
		return Record{}, false, truncated, invalidTimestamp
	}
	return Record{
		SourceKey:      source.Key,
		SourceLabel:    source.Label,
		Path:           candidate.path,
		Text:           builder.String(),
		ModifiedAt:     verified[0].Timestamp,
		TimestampBasis: eventTimestampBasis(verified[0]),
	}, true, truncated, invalidTimestamp
}

func eventTimestampBasis(event sourceEvent) string {
	if event.TimestampBasis != "" {
		return event.TimestampBasis
	}
	return "event_timestamp"
}
