package sources

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiscoverWorklogReportUsesStrictClaudeSchemaAndEventTime(t *testing.T) {
	seoul := time.FixedZone("Asia/Seoul", 9*60*60)
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, seoul)
	root := t.TempDir()
	path := writeSessionFile(t, root, filepath.Join("project", "session.jsonl"), strings.Join([]string{
		`{"type":"user","timestamp":"2026-07-15T14:59:59Z","message":{"role":"user","content":"prior day"}}`,
		`{"type":"user","timestamp":"2026-07-16T14:59:59Z","message":{"role":"user","content":"newest event token=supersecret"}}`,
		`{"type":"assistant","timestamp":"2026-07-16T13:00:00Z","message":{"role":"assistant","content":[{"type":"text","text":"older event"}]}}`,
	}, "\n"), day.AddDate(0, 0, -3))

	report := DiscoverWorklogReport(Source{Key: "claude-code", Label: "Claude Code"}, DiscoverOptions{
		Today: day, PathLookup: []string{root},
	})
	if report.Coverage.Status != "complete" || len(report.Records) != 1 {
		t.Fatalf("unexpected strict Claude report: %#v", report)
	}
	if report.Records[0].Path != path || report.Records[0].TimestampBasis != "event_timestamp" {
		t.Fatalf("unexpected record metadata: %#v", report.Records[0])
	}
	if !strings.HasPrefix(report.Records[0].Text, "newest event") {
		t.Fatalf("newest event must be first: %q", report.Records[0].Text)
	}
	assertContains(t, report.Records[0].Text, safeRedactedMarker())
	assertNotContains(t, report.Records[0].Text, "supersecret")
	assertNotContains(t, report.Records[0].Text, "prior day")
}

func TestDiscoverWorklogReportNeverUsesLegacyGenericFiles(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	writeSessionFile(t, root, filepath.Join("project", "notes.md"), "private arbitrary markdown", day)
	writeSessionFile(t, root, filepath.Join("project", "debug.log"), "private arbitrary log", day)

	report := DiscoverWorklogReport(Source{Key: "claude-code", Label: "Claude Code"}, DiscoverOptions{
		Today: day, PathLookup: []string{root},
	})
	if report.Coverage.Status != "no_activity" || report.Coverage.CandidateFiles != 0 || len(report.Records) != 0 {
		t.Fatalf("strict worklog discovery must not use arbitrary files: %#v", report)
	}

	legacy := DiscoverReport(Source{Key: "test", Label: "Legacy"}, DiscoverOptions{Today: day, PathLookup: []string{root}})
	if len(legacy.Records) != 2 {
		t.Fatalf("legacy summary extractor compatibility changed: %#v", legacy)
	}
}

func TestDiscoverWorklogReportDetectsOpenCodeDatabaseAsUnsupportedStorage(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	writeSessionFile(t, root, "opencode.db", "SQLite format 3", day)

	report := DiscoverWorklogReport(Source{Key: "opencode", Label: "OpenCode"}, DiscoverOptions{
		Today: day, PathLookup: []string{root},
	})
	if report.Coverage.Status != "unsupported_storage" || report.Coverage.CandidateFiles != 1 || len(report.Records) != 0 {
		t.Fatalf("current OpenCode DB must be explicit unsupported storage: %#v", report)
	}
}

func TestDiscoverWorklogReportRejectsUnknownSourceAdapter(t *testing.T) {
	report := DiscoverWorklogReport(Source{Key: "unknown", Label: "Unknown"}, DiscoverOptions{Today: time.Now()})
	if report.Coverage.Status != "unsupported_schema" || len(report.Records) != 0 {
		t.Fatalf("unknown adapters must not use a generic fallback: %#v", report)
	}
}

func TestDiscoverWorklogReportReportsUnknownArtifactSchema(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeSessionFile(t, t.TempDir(), "future.jsonl", `{"type":"future_conversation","timestamp":"2026-07-16T01:00:00Z","payload":{"text":"must not leak"}}`, day)
	report := DiscoverWorklogReport(Source{Key: "claude-code", Label: "Claude Code"}, DiscoverOptions{
		Today: day, PathLookup: []string{path},
	})
	if report.Coverage.Status != "unsupported_schema" || len(report.Records) != 0 {
		t.Fatalf("unknown artifact schema must not look like no activity: %#v", report)
	}
}

func TestDiscoverWorklogReportIncludesArchivedCodexRollouts(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	sessions := filepath.Join(root, "sessions")
	archived := filepath.Join(root, "archived_sessions")
	writeSessionFile(t, archived, "rollout-archived.jsonl", `{"timestamp":"2026-07-16T01:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"archived task"}}`, day)

	report := DiscoverWorklogReport(Source{Key: "codex", Label: "Codex"}, DiscoverOptions{
		Today: day, PathLookup: []string{sessions, archived},
	})
	if report.Coverage.Status != "complete" || len(report.Records) != 1 || !strings.Contains(report.Records[0].Text, "archived task") {
		t.Fatalf("archived Codex rollout was not discovered: %#v", report)
	}
}

func TestWorklogSourcePathsToolHomesReplaceDefaults(t *testing.T) {
	home := t.TempDir()
	lookup := func(key string) string {
		switch key {
		case "CLAUDE_CONFIG_DIR":
			return filepath.Join(home, "claude")
		case "CODEX_HOME":
			return filepath.Join(home, "codex")
		case "XDG_DATA_HOME":
			return filepath.Join(home, "xdg")
		default:
			return ""
		}
	}
	claudeDefault := filepath.Join(home, ".claude", "projects")
	claude := Source{Key: "claude-code", EnvVar: "AI_BRICKLAYING_CLAUDE_DIRS", DefaultPaths: []string{claudeDefault}}
	claudePaths := worklogSourcePaths(claude, DiscoverOptions{EnvLookup: lookup})
	wantClaude := filepath.Join(home, "claude", "projects")
	if len(claudePaths) != 1 || claudePaths[0] != wantClaude {
		t.Fatalf("CLAUDE_CONFIG_DIR must replace defaults, got %v", claudePaths)
	}
	if containsPath(claudePaths, claudeDefault) {
		t.Fatalf("CLAUDE_CONFIG_DIR must not also scan default ~/.claude root: %v", claudePaths)
	}

	codexDefault := filepath.Join(home, ".codex", "sessions")
	codexHome := filepath.Join(home, "codex")
	codex := Source{Key: "codex", EnvVar: "AI_BRICKLAYING_CODEX_DIRS", DefaultPaths: []string{codexDefault, filepath.Join(home, ".codex")}}
	codexPaths := worklogSourcePaths(codex, DiscoverOptions{EnvLookup: lookup})
	wantCodexPaths := []string{filepath.Join(codexHome, "sessions"), filepath.Join(codexHome, "archived_sessions")}
	if len(codexPaths) != len(wantCodexPaths) {
		t.Fatalf("CODEX_HOME must replace defaults, got %v", codexPaths)
	}
	for _, path := range wantCodexPaths {
		if !containsPath(codexPaths, path) {
			t.Fatalf("CODEX_HOME root %q missing: %v", path, codexPaths)
		}
	}
	if containsPath(codexPaths, codexDefault) || containsPath(codexPaths, filepath.Join(home, ".codex")) {
		t.Fatalf("CODEX_HOME must not also scan default ~/.codex roots: %v", codexPaths)
	}

	opencodeDefault := filepath.Join(home, ".local", "share", "opencode")
	opencode := Source{Key: "opencode", EnvVar: "AI_BRICKLAYING_OPENCODE_DIRS", DefaultPaths: []string{opencodeDefault}}
	opencodePaths := worklogSourcePaths(opencode, DiscoverOptions{EnvLookup: lookup})
	wantOpenCode := filepath.Join(home, "xdg", "opencode")
	if len(opencodePaths) != 1 || opencodePaths[0] != wantOpenCode {
		t.Fatalf("XDG_DATA_HOME must replace defaults, got %v", opencodePaths)
	}
	if containsPath(opencodePaths, opencodeDefault) {
		t.Fatalf("XDG_DATA_HOME must not also scan default OpenCode data root: %v", opencodePaths)
	}
}

func TestWorklogSourcePathsExplicitMultiRootOverridesToolHomes(t *testing.T) {
	home := t.TempDir()
	first := filepath.Join(home, "explicit-one")
	second := filepath.Join(home, "explicit-two")
	explicit := strings.Join([]string{first, second}, string(os.PathListSeparator))

	tests := []struct {
		name       string
		source     Source
		toolEnvKey string
	}{
		{
			name:       "Claude Code",
			source:     Source{Key: "claude-code", EnvVar: "AI_BRICKLAYING_CLAUDE_DIRS", DefaultPaths: []string{filepath.Join(home, ".claude", "projects")}},
			toolEnvKey: "CLAUDE_CONFIG_DIR",
		},
		{
			name:       "Codex",
			source:     Source{Key: "codex", EnvVar: "AI_BRICKLAYING_CODEX_DIRS", DefaultPaths: []string{filepath.Join(home, ".codex")}},
			toolEnvKey: "CODEX_HOME",
		},
		{
			name:       "OpenCode",
			source:     Source{Key: "opencode", EnvVar: "AI_BRICKLAYING_OPENCODE_DIRS", DefaultPaths: []string{filepath.Join(home, ".local", "share", "opencode")}},
			toolEnvKey: "XDG_DATA_HOME",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths := worklogSourcePaths(test.source, DiscoverOptions{EnvLookup: func(key string) string {
				switch key {
				case test.source.EnvVar:
					return explicit
				case test.toolEnvKey:
					return filepath.Join(home, "tool-home")
				default:
					return ""
				}
			}})
			if len(paths) != 2 || paths[0] != first || paths[1] != second {
				t.Fatalf("explicit multi-root must take priority, got %v", paths)
			}
		})
	}
}

func TestWorklogDefaultsStayStrictWhenLegacyCatalogUsesBroadRoots(t *testing.T) {
	home := t.TempDir()
	catalog := catalogForPlatform(home, "darwin")

	codex, ok := findInCatalog(catalog, "codex")
	if !ok {
		t.Fatal("codex source missing")
	}
	codexPaths := worklogDefaultPathsAtHome(codex, home)
	wantCodex := []string{filepath.Join(home, ".codex", "sessions"), filepath.Join(home, ".codex", "archived_sessions")}
	if len(codexPaths) != len(wantCodex) || !containsPath(codexPaths, wantCodex[0]) || !containsPath(codexPaths, wantCodex[1]) {
		t.Fatalf("strict Codex roots changed: %v", codexPaths)
	}
	if containsPath(codexPaths, filepath.Join(home, ".codex")) {
		t.Fatalf("strict worklog must not scan whole Codex config tree: %v", codexPaths)
	}

	cursor, ok := findInCatalog(catalog, "cursor")
	if !ok {
		t.Fatal("cursor source missing")
	}
	cursorPaths := worklogDefaultPathsAtHome(cursor, home)
	wantCursor := filepath.Join(home, ".cursor", "projects")
	if len(cursorPaths) != 1 || cursorPaths[0] != wantCursor {
		t.Fatalf("strict Cursor transcript root changed: %v", cursorPaths)
	}
}

func TestCollectAdapterCandidatesUsesGlobalNewestTopKAndDeduplicatesRoots(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "a", "rollout-old.jsonl")
	newest := filepath.Join(root, "z", "rollout-new.jsonl")
	for _, path := range []string{old, newest} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(old, base, base); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newest, base.Add(time.Hour), base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	candidates, truncated, unreadable := collectAdapterCandidates([]string{root, filepath.Join(root, "z")}, 1, matchCodexArtifact)
	if unreadable || !truncated || len(candidates) != 1 || candidates[0].path != newest {
		t.Fatalf("expected global newest unique candidate: candidates=%#v truncated=%t unreadable=%t", candidates, truncated, unreadable)
	}
}

func TestBoundedAdapterCandidatesRetainsOnlyFileLimitEntries(t *testing.T) {
	const (
		fileLimit = 13
		total     = 50_000
	)
	base := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	bounded := newBoundedAdapterCandidates(fileLimit)
	for index := 0; index < total; index++ {
		path := filepath.Join("root", fmt.Sprintf("rollout-%05d.jsonl", index))
		bounded.add(candidate{path: path, modTime: base.Add(time.Duration(index) * time.Second)}, path)
		if len(bounded.entries) > fileLimit || len(bounded.retained) > fileLimit {
			t.Fatalf("candidate memory exceeded file limit at %d: entries=%d retained=%d", index, len(bounded.entries), len(bounded.retained))
		}
	}

	got := bounded.sorted()
	if !bounded.truncated || len(got) != fileLimit || len(bounded.retained) != fileLimit {
		t.Fatalf("unexpected bounded top-k state: candidates=%d retained=%d truncated=%t", len(got), len(bounded.retained), bounded.truncated)
	}
	for offset, value := range got {
		wantIndex := total - 1 - offset
		wantPath := filepath.Join("root", fmt.Sprintf("rollout-%05d.jsonl", wantIndex))
		if value.path != wantPath {
			t.Fatalf("candidate %d = %q, want %q", offset, value.path, wantPath)
		}
	}
}

func TestCollectAdapterCandidatesLargeFixturePreservesTieBreakAndDedupe(t *testing.T) {
	const (
		fileLimit = 17
		total     = 2_048
	)
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	modified := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	for index := 0; index < total; index++ {
		path := filepath.Join(nested, fmt.Sprintf("rollout-%04d.jsonl", index))
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}

	candidates, truncated, unreadable := collectAdapterCandidates([]string{root, nested}, fileLimit, matchCodexArtifact)
	if unreadable || !truncated || len(candidates) != fileLimit {
		t.Fatalf("unexpected large-fixture result: candidates=%d truncated=%t unreadable=%t", len(candidates), truncated, unreadable)
	}
	for offset, value := range candidates {
		wantIndex := total - 1 - offset
		wantPath := filepath.Join(nested, fmt.Sprintf("rollout-%04d.jsonl", wantIndex))
		if value.path != wantPath {
			t.Fatalf("candidate %d = %q, want deterministic tie-break %q", offset, value.path, wantPath)
		}
	}
}

func containsPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	for _, path := range paths {
		if filepath.Clean(path) == want {
			return true
		}
	}
	return false
}

func safeRedactedMarker() string {
	return "[REDACTED]"
}
