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
