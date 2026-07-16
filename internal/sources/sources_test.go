package sources

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"ai-bricklaying/internal/safeio"
)

func TestDiscoverReadsJSONLFromEnvOverrideDirectory(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	path := writeSessionFile(t, root, "session.jsonl", strings.Join([]string{
		`{"message":"Implemented JSONL discovery with token=supersecret and verified records."}`,
		`{"summary":"Captured a reusable session lesson from a local file."}`,
	}, "\n"), today)

	source := Source{Key: "test", Label: "Test", EnvVar: "AI_BRICKLAYING_TEST_DIRS"}
	records := Discover(source, DiscoverOptions{
		Today: today,
		EnvLookup: func(key string) string {
			if key == source.EnvVar {
				return root
			}
			return ""
		},
	})

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Path != path {
		t.Fatalf("expected path %s, got %s", path, records[0].Path)
	}
	assertContains(t, records[0].Text, "Implemented JSONL discovery")
	assertContains(t, records[0].Text, "Captured a reusable session lesson")
	assertContains(t, records[0].Text, safeio.Redacted)
	assertNotContains(t, records[0].Text, "supersecret")
}

func TestDiscoverSupportsDirectFileRoot(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	path := writeSessionFile(t, root, "single.md", "Direct file root produces a session signal.", today)

	records := Discover(Source{Key: "test", Label: "Test"}, DiscoverOptions{Today: today, PathLookup: []string{path}})

	if len(records) != 1 {
		t.Fatalf("expected 1 file-root record, got %d", len(records))
	}
	assertContains(t, records[0].Text, "Direct file root")
}

func TestDiscoverUsesMultipleEnvOverrideRootsWithOSDelimiter(t *testing.T) {
	today := localNoon()
	first := t.TempDir()
	second := t.TempDir()
	writeSessionFile(t, first, "first.txt", "First override root has a useful record.", today)
	writeSessionFile(t, second, "second.log", "Second override root has a useful record.", today)
	source := Source{Key: "test", Label: "Test", EnvVar: "AI_BRICKLAYING_TEST_DIRS"}

	records := Discover(source, DiscoverOptions{
		Today: today,
		EnvLookup: func(key string) string {
			return strings.Join([]string{first, second}, string(os.PathListSeparator))
		},
	})

	if len(records) != 2 {
		t.Fatalf("expected 2 override records, got %d", len(records))
	}
	combined := records[0].Text + "\n" + records[1].Text
	assertContains(t, combined, "First override root")
	assertContains(t, combined, "Second override root")
}

func TestDiscoverSkipsMissingUnreadableAndUnsupportedSources(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	unsupported := writeSessionFile(t, root, "ignored.bin", "ignored", today)
	if runtime.GOOS != "windows" {
		unreadable := filepath.Join(root, "unreadable")
		if err := os.Mkdir(unreadable, 0o000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(unreadable, 0o700)
		_ = unreadable
	}

	records := Discover(Source{Key: "test", Label: "Test"}, DiscoverOptions{Today: today, PathLookup: []string{missing, unsupported, root}})

	if len(records) != 0 {
		t.Fatalf("expected empty successful result, got %#v", records)
	}
}

func TestDiscoverSkipsSymlinkRootsAndDescendants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows setups")
	}
	today := localNoon()
	root := t.TempDir()
	targetRoot := t.TempDir()
	writeSessionFile(t, targetRoot, "target.txt", "Symlink target must not be discovered.", today)
	symlinkRoot := filepath.Join(root, "root-link")
	if err := os.Symlink(targetRoot, symlinkRoot); err != nil {
		t.Fatal(err)
	}
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	symlinkFile := filepath.Join(realDir, "file-link.txt")
	if err := os.Symlink(filepath.Join(targetRoot, "target.txt"), symlinkFile); err != nil {
		t.Fatal(err)
	}

	records := Discover(Source{Key: "test", Label: "Test"}, DiscoverOptions{Today: today, PathLookup: []string{symlinkRoot, root}})

	if len(records) != 0 {
		t.Fatalf("expected symlink paths to be skipped, got %#v", records)
	}
}

func TestDiscoverFiltersByLocalTodayMtime(t *testing.T) {
	today := localNoon()
	yesterday := today.AddDate(0, 0, -1)
	root := t.TempDir()
	writeSessionFile(t, root, "today.txt", "Today record should be present.", today)
	writeSessionFile(t, root, "yesterday.txt", "Yesterday record should be absent.", yesterday)

	records := Discover(Source{Key: "test", Label: "Test"}, DiscoverOptions{Today: today, PathLookup: []string{root}})

	if len(records) != 1 {
		t.Fatalf("expected exactly today's record, got %d", len(records))
	}
	assertContains(t, records[0].Text, "Today record")
	assertNotContains(t, records[0].Text, "Yesterday record")
}

func TestSameLocalDateUsesRequestedDateTimezone(t *testing.T) {
	originalLocal := time.Local
	time.Local = time.UTC
	defer func() { time.Local = originalLocal }()

	seoul := time.FixedZone("Asia/Seoul", 9*60*60)
	requestedDay := time.Date(2026, 7, 17, 0, 0, 0, 0, seoul)
	seoulEarlyMorning := time.Date(2026, 7, 16, 16, 30, 0, 0, time.UTC)
	seoulPriorDay := time.Date(2026, 7, 16, 14, 30, 0, 0, time.UTC)
	if !sameLocalDate(seoulEarlyMorning, requestedDay) {
		t.Fatal("expected UTC timestamp to match requested Seoul date")
	}
	if sameLocalDate(seoulPriorDay, requestedDay) {
		t.Fatal("prior Seoul date must not match requested date")
	}
}

func TestLegacyCodexCatalogPreservesBroadSummaryRoot(t *testing.T) {
	today := localNoon()
	home := t.TempDir()
	source, ok := findInCatalog(catalogForPlatform(home, "darwin"), "codex")
	if !ok {
		t.Fatal("codex source missing")
	}
	broadRoot := filepath.Join(home, ".codex")
	if !containsPath(source.DefaultPaths, filepath.Join(broadRoot, "sessions")) || !containsPath(source.DefaultPaths, broadRoot) {
		t.Fatalf("legacy Codex roots changed: %v", source.DefaultPaths)
	}
	writeSessionFile(t, broadRoot, "legacy-summary.txt", "Legacy Codex single-source summary remains discoverable.", today)

	records := Discover(source, DiscoverOptions{Today: today, EnvLookup: func(string) string { return "" }})
	if len(records) != 1 {
		t.Fatalf("expected legacy Codex summary record, got %#v", records)
	}
	assertContains(t, records[0].Text, "Legacy Codex single-source summary")
}

func TestLegacyCursorCatalogPreservesWorkspaceStorageSummaryRoot(t *testing.T) {
	today := localNoon()
	home := t.TempDir()
	source, ok := findInCatalog(catalogForPlatform(home, "darwin"), "cursor")
	if !ok {
		t.Fatal("cursor source missing")
	}
	wantRoot := filepath.Join(home, "Library/Application Support/Cursor/User/workspaceStorage")
	if len(source.DefaultPaths) != 1 || source.DefaultPaths[0] != wantRoot {
		t.Fatalf("legacy Cursor root changed: %v", source.DefaultPaths)
	}
	writeSessionFile(t, wantRoot, filepath.Join("workspace", "legacy-summary.txt"), "Legacy Cursor single-source summary remains discoverable.", today)

	records := Discover(source, DiscoverOptions{Today: today, EnvLookup: func(string) string { return "" }})
	if len(records) != 1 {
		t.Fatalf("expected legacy Cursor summary record, got %#v", records)
	}
	assertContains(t, records[0].Text, "Legacy Cursor single-source summary")
}

func TestDiscoverExtractsOnlyAllowlistedCodexMessages(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	writeSessionFile(t, root, "rollout.jsonl", strings.Join([]string{
		`{"timestamp":"2026-07-16T09:00:00Z","type":"session_meta","payload":{"base_instructions":"EXCLUDED base instructions","cwd":"EXCLUDED session metadata"}}`,
		`{"timestamp":"2026-07-16T09:01:00Z","type":"turn_context","payload":{"developer_instructions":"EXCLUDED developer instructions"}}`,
		`{"timestamp":"2026-07-16T09:02:00Z","type":"response_item","payload":{"type":"message","role":"system","content":[{"type":"input_text","text":"EXCLUDED system message"}]}}`,
		`{"timestamp":"2026-07-16T09:03:00Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"EXCLUDED developer message"}]}}`,
		`{"timestamp":"2026-07-16T09:04:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Add Linux source discovery."},{"type":"input_image","text":"EXCLUDED image metadata"}]}}`,
		`{"timestamp":"2026-07-16T09:05:00Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Implemented platform-specific roots."},{"type":"reasoning","text":"EXCLUDED private reasoning"}]}}`,
		`{"timestamp":"2026-07-16T09:06:00Z","type":"event_msg","payload":{"type":"user_message","message":"Please run the source tests."}}`,
		`{"timestamp":"2026-07-16T09:07:00Z","type":"event_msg","payload":{"type":"agent_message","message":"All source tests passed."}}`,
		`{"timestamp":"2026-07-16T09:08:00Z","type":"event_msg","payload":{"type":"agent_reasoning","message":"EXCLUDED agent reasoning"}}`,
		`{"timestamp":"2026-07-16T09:09:00Z","type":"response_item","payload":{"type":"function_call_output","output":"EXCLUDED tool output","content":[{"type":"output_text","text":"EXCLUDED nested tool text"}]}}`,
	}, "\n"), today)

	report := DiscoverReport(Source{Key: "codex", Label: "Codex"}, DiscoverOptions{
		Today:      today,
		PathLookup: []string{root},
	})

	if report.Coverage.Status != "complete" || len(report.Records) != 1 {
		t.Fatalf("expected one complete Codex record, got %#v", report)
	}
	for _, expected := range []string{
		"Add Linux source discovery.",
		"Implemented platform-specific roots.",
		"Please run the source tests.",
		"All source tests passed.",
	} {
		assertContains(t, report.Records[0].Text, expected)
	}
	for _, excluded := range []string{
		"EXCLUDED base instructions",
		"EXCLUDED session metadata",
		"EXCLUDED developer instructions",
		"EXCLUDED system message",
		"EXCLUDED developer message",
		"EXCLUDED image metadata",
		"EXCLUDED private reasoning",
		"EXCLUDED agent reasoning",
		"EXCLUDED tool output",
		"EXCLUDED nested tool text",
	} {
		assertNotContains(t, report.Records[0].Text, excluded)
	}
}

func TestDiscoverDoesNotTreatCodexMetadataAsActivity(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	writeSessionFile(t, root, "metadata-only.jsonl", strings.Join([]string{
		`{"timestamp":"2026-07-16T09:00:00Z","type":"session_meta","payload":{"base_instructions":"Never collect this instruction."}}`,
		`{"timestamp":"2026-07-16T09:01:00Z","type":"response_item","payload":{"type":"message","role":"system","content":[{"type":"input_text","text":"Never collect this system message."}]}}`,
		`{"timestamp":"2026-07-16T09:02:00Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"Never collect this developer message."}]}}`,
	}, "\n"), today)

	report := DiscoverReport(Source{Key: "codex", Label: "Codex"}, DiscoverOptions{
		Today:      today,
		PathLookup: []string{root},
	})

	if report.Coverage.Status != "no_activity" || len(report.Records) != 0 {
		t.Fatalf("Codex metadata must not become activity: %#v", report)
	}
}

func TestCandidateLimitAppliesAfterDateFilterAndKeepsNewest(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	for index := 0; index < 5; index++ {
		writeSessionFile(t, root, fmt.Sprintf("z-old-%d.txt", index), "old archive", today.AddDate(0, 0, -1))
	}
	writeSessionFile(t, root, "z-today-older.txt", "older today", today.Add(-time.Hour))
	newest := writeSessionFile(t, root, "a-today-newest.txt", "newest today", today.Add(time.Hour))

	report := DiscoverReport(Source{Key: "test", Label: "Test"}, DiscoverOptions{
		Today: today, PathLookup: []string{root}, FileLimit: 1,
	})

	if len(report.Records) != 1 || report.Records[0].Path != newest {
		t.Fatalf("candidate cap did not preserve newest current-day record: %#v", report.Records)
	}
	if report.Coverage.Status != "truncated" {
		t.Fatalf("expected truncated coverage for two current-day records, got %#v", report.Coverage)
	}
}

func TestUnreadableExistingRootIsNotReportedAsNoActivity(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	if _, err := os.ReadDir(root); err == nil {
		t.Skip("current user can still read mode-000 directories")
	}

	report := DiscoverReport(Source{Key: "test", Label: "Test"}, DiscoverOptions{Today: today, PathLookup: []string{root}})
	if report.Coverage.Status != "unreadable" {
		t.Fatalf("unreadable root reported as %#v", report.Coverage)
	}
}

func TestDiscoverEnforcesRecordAndFileLimits(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	for index := 0; index < 14; index++ {
		writeSessionFile(t, root, fmt.Sprintf("session-%02d.txt", index), fmt.Sprintf("record %02d %s", index, strings.Repeat("x", 50)), today.Add(time.Duration(index)*time.Minute))
	}
	large := writeSessionFile(t, root, "newest.txt", strings.Repeat("a", 25_000), today.Add(30*time.Minute))

	records := Discover(Source{Key: "test", Label: "Test"}, DiscoverOptions{Today: today, PathLookup: []string{root}})

	if len(records) != DefaultRecordLimit {
		t.Fatalf("expected %d records, got %d", DefaultRecordLimit, len(records))
	}
	if records[0].Path != large {
		t.Fatalf("expected newest file first, got %s", records[0].Path)
	}
	if len([]rune(records[0].Text)) != DefaultMaxChars {
		t.Fatalf("expected large file to be truncated to %d chars, got %d", DefaultMaxChars, len([]rune(records[0].Text)))
	}
}

func TestDiscoverExtractsConfiguredJSONKeys(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	writeSessionFile(t, root, "keys.json", `{
		"text":"Text key value",
		"content":["Content key value", {"message":"Message key value"}],
		"prompt":"Prompt key value",
		"response":"Response key value",
		"summary":"Summary key value",
		"ignored":"Ignored key value"
	}`, today)

	records := Discover(Source{Key: "test", Label: "Test"}, DiscoverOptions{Today: today, PathLookup: []string{root}})

	if len(records) != 1 {
		t.Fatalf("expected 1 JSON record, got %d", len(records))
	}
	for _, expected := range []string{"Text key value", "Content key value", "Message key value", "Prompt key value", "Response key value", "Summary key value"} {
		assertContains(t, records[0].Text, expected)
	}
	assertNotContains(t, records[0].Text, "Ignored key value")
}

func TestDiscoverParsesJSONBeforeApplyingExcerptLimit(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	writeSessionFile(t, root, "large-valid.json", fmt.Sprintf(`{"message":"Useful extracted message.","ignored":%q}`, strings.Repeat("ignored-metadata-", 2_000)), today)

	report := DiscoverReport(Source{Key: "test", Label: "Test"}, DiscoverOptions{
		Today:      today,
		PathLookup: []string{root},
		MaxChars:   80,
	})

	if report.Coverage.Status != "complete" {
		t.Fatalf("expected complete coverage after parsing full JSON, got %#v", report.Coverage)
	}
	if len(report.Records) != 1 {
		t.Fatalf("expected 1 extracted record, got %#v", report.Records)
	}
	if report.Records[0].Text != "Useful extracted message." {
		t.Fatalf("unexpected extracted text: %q", report.Records[0].Text)
	}
}

func TestDiscoverReportsMalformedJSONWithoutRawFallback(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	writeSessionFile(t, root, "malformed.json", `{"message":"raw malformed payload","password":"must-not-leak"`, today)

	report := DiscoverReport(Source{Key: "test", Label: "Test"}, DiscoverOptions{Today: today, PathLookup: []string{root}})

	if report.Coverage.Status != "parse_error" {
		t.Fatalf("expected parse_error coverage, got %#v", report.Coverage)
	}
	if len(report.Records) != 0 {
		t.Fatalf("malformed JSON must not fall back to raw evidence: %#v", report.Records)
	}
}

func TestDiscoverJSONLSkipsMalformedLinesAndReportsParseError(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	writeSessionFile(t, root, "mixed.jsonl", strings.Join([]string{
		`{"message":"Valid parsed evidence."}`,
		`{"message":"malformed raw evidence token=must-not-leak"`,
	}, "\n"), today)

	report := DiscoverReport(Source{Key: "test", Label: "Test"}, DiscoverOptions{Today: today, PathLookup: []string{root}})

	if report.Coverage.Status != "parse_error" {
		t.Fatalf("expected parse_error coverage, got %#v", report.Coverage)
	}
	if len(report.Records) != 1 {
		t.Fatalf("expected valid JSONL lines to remain usable, got %#v", report.Records)
	}
	assertContains(t, report.Records[0].Text, "Valid parsed evidence")
	assertNotContains(t, report.Records[0].Text, "must-not-leak")
}

func TestOversizedJSONLKeepsLatestCompleteRecords(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	oversizedOldLine := `{"message":"` + strings.Repeat("old-record-padding-", DefaultScanFileBytes/19+1) + `"}`
	latest := `{"message":"Latest complete work evidence."}`
	writeSessionFile(t, root, "large-session.jsonl", oversizedOldLine+"\n"+latest+"\n", today)

	report := DiscoverReport(Source{Key: "test", Label: "Test"}, DiscoverOptions{Today: today, PathLookup: []string{root}})
	if report.Coverage.Status != "truncated" {
		t.Fatalf("expected truncated coverage, got %#v", report.Coverage)
	}
	if len(report.Records) != 1 {
		t.Fatalf("expected latest complete JSONL evidence, got %#v", report.Records)
	}
	assertContains(t, report.Records[0].Text, "Latest complete work evidence")
	assertNotContains(t, report.Records[0].Text, "old-record-padding")
}

func TestOversizedTrailingJSONLLineKeepsEarlyCompleteRecord(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	early := `{"timestamp":"2026-07-16T09:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Early complete work evidence."}]}}`
	oversized := `{"timestamp":"2026-07-16T09:01:00Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OVERSIZED-RAW-` + strings.Repeat("private-trailing-payload-", DefaultScanFileBytes/24+1) + `"}]}}`
	writeSessionFile(t, root, "oversized-trailing.jsonl", early+"\n"+oversized+"\n", today)

	report := DiscoverReport(Source{Key: "codex", Label: "Codex"}, DiscoverOptions{
		Today:      today,
		PathLookup: []string{root},
	})

	if report.Coverage.Status != "truncated" || len(report.Records) != 1 {
		t.Fatalf("expected early evidence with truncated coverage, got %#v", report)
	}
	assertContains(t, report.Records[0].Text, "Early complete work evidence.")
	assertNotContains(t, report.Records[0].Text, "OVERSIZED-RAW")
	assertNotContains(t, report.Records[0].Text, "private-trailing-payload")
}

func TestDiscoverDeduplicatesCandidatesFromNestedRoots(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	path := writeSessionFile(t, nested, "session.txt", "A nested source record.", today)

	report := DiscoverReport(Source{Key: "test", Label: "Test"}, DiscoverOptions{
		Today:      today,
		PathLookup: []string{nested, root, nested},
	})

	if report.Coverage.CandidateFiles != 1 || report.Coverage.UsedRecords != 1 {
		t.Fatalf("expected one unique candidate and record, got %#v", report.Coverage)
	}
	if len(report.Records) != 1 || report.Records[0].Path != path {
		t.Fatalf("expected one unique nested record, got %#v", report.Records)
	}
}

func TestDiscoverBoundsCandidateFileScan(t *testing.T) {
	today := localNoon()
	root := t.TempDir()
	for index := 0; index < 3; index++ {
		writeSessionFile(t, root, fmt.Sprintf("session-%d.txt", index), fmt.Sprintf("record %d", index), today)
	}

	report := DiscoverReport(Source{Key: "test", Label: "Test"}, DiscoverOptions{
		Today:      today,
		PathLookup: []string{root},
		FileLimit:  2,
	})

	if report.Coverage.Status != "truncated" {
		t.Fatalf("expected truncated coverage at file cap, got %#v", report.Coverage)
	}
	if report.Coverage.CandidateFiles != 2 || report.Coverage.UsedRecords != 2 {
		t.Fatalf("expected two bounded candidates, got %#v", report.Coverage)
	}
}

func TestCatalogIncludesDocumentedSources(t *testing.T) {
	expected := map[string]string{
		"opencode":       "AI_BRICKLAYING_OPENCODE_DIRS",
		"claude-code":    "AI_BRICKLAYING_CLAUDE_DIRS",
		"codex":          "AI_BRICKLAYING_CODEX_DIRS",
		"cursor":         "AI_BRICKLAYING_CURSOR_DIRS",
		"github-copilot": "AI_BRICKLAYING_COPILOT_DIRS",
	}
	for _, source := range Catalog() {
		if expected[source.Key] != source.EnvVar {
			t.Fatalf("unexpected source catalog entry: %#v", source)
		}
		delete(expected, source.Key)
	}
	if len(expected) > 0 {
		t.Fatalf("missing source catalog entries: %#v", expected)
	}
}

func TestLegacyCatalogUsesPlatformSpecificEditorSourceRoots(t *testing.T) {
	home := filepath.Join("test", "home")
	tests := []struct {
		name string
		goos string
		key  string
		want string
	}{
		{
			name: "darwin cursor",
			goos: "darwin",
			key:  "cursor",
			want: filepath.Join(home, "Library/Application Support/Cursor/User/workspaceStorage"),
		},
		{
			name: "darwin github copilot",
			goos: "darwin",
			key:  "github-copilot",
			want: filepath.Join(home, "Library/Application Support/Code/User/workspaceStorage"),
		},
		{
			name: "linux cursor",
			goos: "linux",
			key:  "cursor",
			want: filepath.Join(home, ".config/Cursor/User/workspaceStorage"),
		},
		{
			name: "linux github copilot",
			goos: "linux",
			key:  "github-copilot",
			want: filepath.Join(home, ".config/Code/User/workspaceStorage"),
		},
		{
			name: "windows cursor",
			goos: "windows",
			key:  "cursor",
			want: filepath.Join(home, "AppData/Roaming/Cursor/User/workspaceStorage"),
		},
		{
			name: "windows github copilot",
			goos: "windows",
			key:  "github-copilot",
			want: filepath.Join(home, "AppData/Roaming/Code/User/workspaceStorage"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, ok := findInCatalog(catalogForPlatform(home, test.goos), test.key)
			if !ok {
				t.Fatalf("source %q missing", test.key)
			}
			if len(source.DefaultPaths) != 1 || source.DefaultPaths[0] != test.want {
				t.Fatalf("unexpected paths for %s on %s: %v", test.key, test.goos, source.DefaultPaths)
			}
		})
	}
}

func findInCatalog(catalog []Source, key string) (Source, bool) {
	for _, source := range catalog {
		if source.Key == key {
			return source, true
		}
	}
	return Source{}, false
}

func localNoon() time.Time {
	now := time.Now().Local()
	year, month, day := now.Date()
	return time.Date(year, month, day, 12, 0, 0, 0, time.Local)
}

func writeSessionFile(t *testing.T, root string, name string, contents string, modTime time.Time) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertContains(t *testing.T, value string, expected string) {
	t.Helper()
	if !strings.Contains(value, expected) {
		t.Fatalf("expected %q to contain %q", value, expected)
	}
}

func assertNotContains(t *testing.T, value string, forbidden string) {
	t.Helper()
	if strings.Contains(value, forbidden) {
		t.Fatalf("expected %q not to contain %q", value, forbidden)
	}
}
