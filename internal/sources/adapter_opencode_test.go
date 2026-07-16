package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMatchOpenCodeArtifactScopesDatabaseAndLegacyParts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "opencode")
	tests := []struct {
		name   string
		path   string
		direct bool
		want   bool
	}{
		{name: "current database", path: filepath.Join(root, "opencode.db"), want: true},
		{name: "database sidecar", path: filepath.Join(root, "opencode.db-wal"), want: false},
		{name: "nested database", path: filepath.Join(root, "backup", "opencode.db"), want: false},
		{name: "legacy part", path: filepath.Join(root, "storage", "part", "msg_123", "prt_456.json"), want: true},
		{name: "legacy non part", path: filepath.Join(root, "storage", "message", "ses_123", "msg_456.json"), want: false},
		{name: "pre migration part", path: filepath.Join(root, "project", "project-id", "storage", "session", "part", "ses_123", "msg_456", "prt_789.json"), want: true},
		{name: "bad part id", path: filepath.Join(root, "storage", "part", "message", "part.json"), want: false},
		{name: "auth", path: filepath.Join(root, "auth.json"), want: false},
		{name: "log", path: filepath.Join(root, "log", "session.jsonl"), want: false},
		{name: "arbitrary json", path: filepath.Join(root, "data.json"), want: false},
		{name: "outside root", path: filepath.Join(filepath.Dir(root), "outside", "storage", "part", "msg_123", "prt_456.json"), want: false},
		{name: "direct exported json", path: filepath.Join(root, "session-export.json"), direct: true, want: true},
		{name: "direct event jsonl", path: filepath.Join(root, "session-events.jsonl"), direct: true, want: true},
		{name: "direct database", path: filepath.Join(root, "opencode.db"), direct: true, want: true},
		{name: "direct auth stays private", path: filepath.Join(root, "auth.json"), direct: true, want: false},
		{name: "direct log stays private", path: filepath.Join(root, "logs", "session.jsonl"), direct: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matchRoot := root
			if test.direct {
				matchRoot = test.path
			}
			if got := matchOpenCodeArtifact(matchRoot, test.path, test.direct); got != test.want {
				t.Fatalf("matchOpenCodeArtifact(%q, %q, %t) = %t, want %t", matchRoot, test.path, test.direct, got, test.want)
			}
		})
	}
}

func TestParseOpenCodeArtifactReportsCurrentDatabaseWithoutReadingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.db")
	if err := os.WriteFile(path, []byte("not even a sqlite database; must not be parsed"), 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o600)

	result := parseOpenCodeArtifact(path, time.Now())
	if !result.UnsupportedStorage {
		t.Fatalf("opencode.db must be reported as unsupported storage: %#v", result)
	}
	if result.ParseError || result.Truncated || result.Unreadable || result.UnsupportedSchema || len(result.Events) != 0 {
		t.Fatalf("database detection must not inspect or misclassify content: %#v", result)
	}
}

func TestDiscoverWorklogReportOpenCodeDistinguishesDatabaseFromPrivateFiles(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	databaseRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(databaseRoot, "opencode.db"), []byte("opaque database"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := DiscoverWorklogReport(Source{Key: "opencode", Label: "OpenCode"}, DiscoverOptions{
		Today: day, PathLookup: []string{databaseRoot},
	})
	if report.Coverage.Status != "unsupported_storage" || report.Coverage.CandidateFiles != 1 || len(report.Records) != 0 {
		t.Fatalf("current database must be explicit unsupported storage: %#v", report)
	}

	privateRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(privateRoot, "auth.json"), []byte(`{"token":"must stay private"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(privateRoot, "data.json"), []byte(`{"text":"must not be scraped"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	privateReport := DiscoverWorklogReport(Source{Key: "opencode", Label: "OpenCode"}, DiscoverOptions{
		Today: day, PathLookup: []string{privateRoot},
	})
	if privateReport.Coverage.Status != "no_activity" || privateReport.Coverage.CandidateFiles != 0 || len(privateReport.Records) != 0 {
		t.Fatalf("auth and generic data files must not become candidates: %#v", privateReport)
	}
}

func TestParseOpenCodeArtifactUsesTrustedEventTimestampAndMtimeFallback(t *testing.T) {
	seoul := time.FixedZone("Asia/Seoul", 9*60*60)
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, seoul)
	root := t.TempDir()

	inDay := writeOpenCodeJSON(t, root, "in-day.json", `{
  "id":"prt_in_day","messageID":"msg_1","sessionID":"ses_1","type":"text",
  "text":"trusted in-day event","time":{"start":1784156400000},
  "metadata":{"private":"must not be copied"}
}`, day.AddDate(0, 0, -3))
	outOfDay := writeOpenCodeJSON(t, root, "out-of-day.json", `{
  "id":"prt_out_day","messageID":"msg_1","sessionID":"ses_1","type":"text",
  "text":"trusted previous-day event","time":{"start":1784070000000}
}`, day)
	fallback := writeOpenCodeJSON(t, root, "fallback.json", `{
  "id":"prt_fallback","messageID":"msg_1","sessionID":"ses_1","type":"text",
  "text":"mtime fallback event"
}`, day)

	first := parseOpenCodeArtifact(inDay, day)
	if got := openCodeEventTexts(first.Events); got != "trusted in-day event" {
		t.Fatalf("trusted timestamp must select in-day event, got %q (%#v)", got, first)
	}
	second := parseOpenCodeArtifact(outOfDay, day)
	if len(second.Events) != 0 {
		t.Fatalf("today mtime must not override an out-of-day event timestamp: %#v", second)
	}
	third := parseOpenCodeArtifact(fallback, day)
	if got := openCodeEventTexts(third.Events); got != "mtime fallback event" {
		t.Fatalf("strict legacy part may use mtime fallback, got %q (%#v)", got, third)
	}
	if len(third.Events) != 1 || third.Events[0].TimestampBasis != "file_modified_at" {
		t.Fatalf("mtime fallback must be explicit in event metadata: %#v", third.Events)
	}
	if len(first.Events) != 1 || first.Events[0].TimestampBasis != "" {
		t.Fatalf("trusted event timestamp must keep the default basis: %#v", first.Events)
	}
	if strings.Contains(openCodeEventTexts(first.Events), "must not be copied") {
		t.Fatal("part metadata must never become evidence")
	}
}

func TestParseOpenCodeJSONLAllowsOnlyCompletedTextEvents(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeOpenCodeJSONL(t, []string{
		`{"type":"step_start","timestamp":1784163600000,"sessionID":"ses_1","part":{"id":"prt_step","messageID":"msg_1","sessionID":"ses_1","type":"step-start","snapshot":"EXCLUDED snapshot"}}`,
		`{"type":"tool_use","timestamp":1784167200000,"sessionID":"ses_1","part":{"id":"prt_tool","messageID":"msg_1","sessionID":"ses_1","type":"tool","state":{"input":{"password":"EXCLUDED tool input"},"output":"EXCLUDED tool output"}}}`,
		`{"type":"reasoning","timestamp":1784170800000,"sessionID":"ses_1","part":{"id":"prt_reason","messageID":"msg_1","sessionID":"ses_1","type":"reasoning","text":"EXCLUDED reasoning"}}`,
		`{"type":"text","timestamp":1784174400000,"sessionID":"ses_1","part":{"id":"prt_text","messageID":"msg_1","sessionID":"ses_1","type":"text","text":"completed assistant response","time":{"end":1784174400000},"metadata":{"authorization":"EXCLUDED metadata"}}}`,
		`{"type":"text","timestamp":1784178000000,"sessionID":"ses_1","part":{"id":"prt_synthetic","messageID":"msg_1","sessionID":"ses_1","type":"text","text":"EXCLUDED synthetic","synthetic":true}}`,
		`{"type":"error","timestamp":1784181600000,"sessionID":"ses_1","error":{"data":{"message":"EXCLUDED provider error"}}}`,
	})

	result := parseOpenCodeArtifact(path, day)
	if result.ParseError || result.Truncated || result.Unreadable || result.UnsupportedSchema || result.UnsupportedStorage {
		t.Fatalf("unexpected parse flags: %#v", result)
	}
	if got := openCodeEventTexts(result.Events); got != "completed assistant response" {
		t.Fatalf("only visible text events may be collected, got %q", got)
	}
	for _, excluded := range []string{"snapshot", "tool input", "tool output", "reasoning", "metadata", "synthetic", "provider error"} {
		if strings.Contains(openCodeEventTexts(result.Events), excluded) {
			t.Fatalf("private %q payload leaked: %#v", excluded, result.Events)
		}
	}
}

func TestParseOpenCodeMessageExportRequiresMatchingConversationIdentity(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeOpenCodeJSON(t, t.TempDir(), "session-export.json", `{
  "info":{"id":"ses_export","title":"EXCLUDED session title","time":{"created":1784160000000}},
  "messages":[
    {"info":{"id":"msg_user","sessionID":"ses_export","role":"user","time":{"created":1784163600000}},"parts":[
      {"id":"prt_user","messageID":"msg_user","sessionID":"ses_export","type":"text","text":"user request"},
      {"id":"prt_file","messageID":"msg_user","sessionID":"ses_export","type":"file","url":"EXCLUDED file URL"}
    ]},
    {"info":{"id":"msg_assistant","sessionID":"ses_export","role":"assistant","time":{"created":1784167200000}},"parts":[
      {"id":"prt_answer","messageID":"msg_assistant","sessionID":"ses_export","type":"text","text":"assistant answer"},
      {"id":"prt_tool","messageID":"msg_assistant","sessionID":"ses_export","type":"tool","state":{"output":"EXCLUDED tool output"}}
    ]}
  ]
}`, day)

	result := parseOpenCodeArtifact(path, day)
	if result.ParseError || result.Truncated || result.Unreadable || result.UnsupportedSchema || result.UnsupportedStorage {
		t.Fatalf("unexpected export flags: %#v", result)
	}
	texts := openCodeEventTexts(result.Events)
	for _, expected := range []string{"user request", "assistant answer"} {
		if !strings.Contains(texts, expected) {
			t.Fatalf("expected %q in %q", expected, texts)
		}
	}
	for _, excluded := range []string{"session title", "file URL", "tool output"} {
		if strings.Contains(texts, excluded) {
			t.Fatalf("did not expect %q in %q", excluded, texts)
		}
	}
	if len(result.Events) != 2 || result.Events[1].Sequence <= result.Events[0].Sequence {
		t.Fatalf("export event order must be stable: %#v", result.Events)
	}
}

func TestParseOpenCodeArtifactRejectsUnknownSchemaWithoutGenericTextLeak(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeOpenCodeJSON(t, t.TempDir(), "unknown.json", `{
  "type":"conversation",
  "timestamp":1784163600000,
  "message":"EXCLUDED generic message",
  "content":"EXCLUDED generic content",
  "text":"EXCLUDED generic text",
  "auth":{"token":"EXCLUDED token"},
  "path":"/EXCLUDED/private/path"
}`, day)

	result := parseOpenCodeArtifact(path, day)
	if !result.UnsupportedSchema || len(result.Events) != 0 {
		t.Fatalf("unknown JSON must be unsupported without generic extraction: %#v", result)
	}
	if result.ParseError || result.Unreadable || result.UnsupportedStorage {
		t.Fatalf("unknown schema is distinct from parse/storage failure: %#v", result)
	}
}

func TestParseOpenCodeArtifactRejectsLookalikeSessionAndUnknownPartType(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	lookalike := writeOpenCodeJSON(t, t.TempDir(), "lookalike.json", `{
  "id":"ses_lookalike","time":{},"title":"EXCLUDED title","text":"EXCLUDED text"
}`, day)
	unknownPart := writeOpenCodeJSON(t, t.TempDir(), "unknown-part.json", `{
  "id":"prt_unknown","messageID":"msg_1","sessionID":"ses_1","type":"credential",
  "text":"EXCLUDED credential payload","time":{"start":1784163600000}
}`, day)

	for _, path := range []string{lookalike, unknownPart} {
		result := parseOpenCodeArtifact(path, day)
		if !result.UnsupportedSchema || len(result.Events) != 0 {
			t.Fatalf("OpenCode lookalike must be unsupported without evidence: %#v", result)
		}
	}
}

func TestParseOpenCodeArtifactReportsMalformedJSONLWithoutRawFallback(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeOpenCodeJSONL(t, []string{
		`{"type":"text","timestamp":1784163600000,"sessionID":"ses_1","part":{"id":"prt_good","messageID":"msg_1","sessionID":"ses_1","type":"text","text":"valid event"}}`,
		`{"type":"text","text":"EXCLUDED malformed secret`,
	})

	result := parseOpenCodeArtifact(path, day)
	if !result.ParseError {
		t.Fatalf("malformed JSONL must set ParseError: %#v", result)
	}
	if got := openCodeEventTexts(result.Events); got != "valid event" {
		t.Fatalf("malformed raw payload must not leak, got %q", got)
	}
}

func TestParseOpenCodeArtifactReportsUnreadableFile(t *testing.T) {
	result := parseOpenCodeArtifact(filepath.Join(t.TempDir(), "missing.json"), time.Now())
	if !result.Unreadable || len(result.Events) != 0 {
		t.Fatalf("missing artifact must be unreadable without events: %#v", result)
	}
}

func writeOpenCodeJSON(t *testing.T, root string, name string, content string, modTime time.Time) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeOpenCodeJSONL(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func openCodeEventTexts(events []sourceEvent) string {
	texts := make([]string, 0, len(events))
	for _, event := range events {
		texts = append(texts, event.Text)
	}
	return strings.Join(texts, "\n")
}
