package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMatchCursorArtifactScopesAgentTranscriptJSONL(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".cursor", "projects")
	project := filepath.Join(root, "Users-example-project")
	transcripts := filepath.Join(project, "agent-transcripts")

	tests := []struct {
		name   string
		path   string
		direct bool
		want   bool
	}{
		{name: "main transcript", path: filepath.Join(transcripts, "session", "session.jsonl"), want: true},
		{name: "flat transcript", path: filepath.Join(transcripts, "session.jsonl"), want: true},
		{name: "subagent transcript", path: filepath.Join(transcripts, "session", "subagents", "agent.jsonl"), want: true},
		{name: "legacy text transcript", path: filepath.Join(transcripts, "session.txt"), want: false},
		{name: "project metadata", path: filepath.Join(project, "repo.json"), want: false},
		{name: "similar directory name", path: filepath.Join(project, "agent-transcripts-old", "session.jsonl"), want: false},
		{name: "workspace storage", path: filepath.Join(root, "workspaceStorage", "agent-transcripts", "session.jsonl"), want: false},
		{name: "outside root", path: filepath.Join(filepath.Dir(root), "outside", "agent-transcripts", "session.jsonl"), want: false},
		{name: "direct jsonl", path: filepath.Join(root, "copied-session.jsonl"), direct: true, want: true},
		{name: "direct non-jsonl", path: filepath.Join(root, "copied-session.json"), direct: true, want: false},
		{name: "direct path mismatch", path: filepath.Join(root, "other.jsonl"), direct: true, want: false},
		{name: "direct workspace storage", path: filepath.Join(root, "workspaceStorage", "copied.jsonl"), direct: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matchRoot := root
			if test.direct {
				matchRoot = filepath.Join(root, "copied-session.jsonl")
				if test.name == "direct workspace storage" {
					matchRoot = test.path
				}
			}
			if got := matchCursorArtifact(matchRoot, test.path, test.direct); got != test.want {
				t.Fatalf("matchCursorArtifact(%q, %q, %t) = %t, want %t", matchRoot, test.path, test.direct, got, test.want)
			}
		})
	}
}

func TestParseCursorArtifactReadsRealShapeAndUnwrapsUserQuery(t *testing.T) {
	seoul := time.FixedZone("Asia/Seoul", 9*60*60)
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, seoul)
	path := writeCursorArtifact(t, []string{
		`{"role":"user","message":{"content":[{"type":"text","text":"<user_query>\nImplement the Cursor adapter.\n</user_query>"}]}}`,
		`{"role":"assistant","message":{"content":[{"type":"text","text":"Implemented strict transcript parsing."}]}}`,
	}, time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC))

	result := parseCursorArtifact(path, day)
	assertCleanCursorResult(t, result)
	texts := cursorEventTexts(result.Events)
	if !strings.Contains(texts, "Implement the Cursor adapter.") {
		t.Fatalf("expected wrapped user query in events: %q", texts)
	}
	if strings.Contains(texts, "Implemented strict transcript parsing.") {
		t.Fatalf("assistant text must not become strict Cursor evidence: %q", texts)
	}
	if strings.Contains(texts, cursorUserQueryOpen) || strings.Contains(texts, cursorUserQueryClose) {
		t.Fatalf("exact user_query wrapper should be removed: %q", texts)
	}
	for _, event := range result.Events {
		if !event.Timestamp.Equal(time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)) {
			t.Fatalf("timestamp-less Cursor events must use file mtime, got %s", event.Timestamp)
		}
		if event.TimestampBasis != "file_modified_at" {
			t.Fatalf("mtime fallback must be explicit, got basis %q", event.TimestampBasis)
		}
	}
}

func TestParseCursorArtifactUsesEventTimestampAndRequestedTimezone(t *testing.T) {
	seoul := time.FixedZone("Asia/Seoul", 9*60*60)
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, seoul)
	path := writeCursorArtifact(t, []string{
		`{"type":"message","timestamp":"2026-07-15T14:59:59Z","message":{"role":"user","content":[{"type":"text","text":"<user_query>previous day</user_query>"}]}}`,
		`{"type":"message","timestamp":"2026-07-15T15:00:00Z","message":{"role":"user","content":[{"type":"text","text":"<user_query>start boundary</user_query>"}]}}`,
		`{"type":"message","timestamp":"2026-07-16T14:59:59Z","message":{"role":"user","content":[{"type":"text","text":"<user_query>end boundary</user_query>"}]}}`,
		`{"type":"message","timestamp":"2026-07-16T15:00:00Z","message":{"role":"user","content":[{"type":"text","text":"<user_query>next day</user_query>"}]}}`,
	}, time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC))

	result := parseCursorArtifact(path, day)
	assertCleanCursorResult(t, result)
	texts := cursorEventTexts(result.Events)
	for _, expected := range []string{"start boundary", "end boundary"} {
		if !strings.Contains(texts, expected) {
			t.Fatalf("expected %q in events: %q", expected, texts)
		}
	}
	for _, excluded := range []string{"previous day", "next day"} {
		if strings.Contains(texts, excluded) {
			t.Fatalf("event timestamp outside requested date must not use mtime fallback: %q", texts)
		}
	}
	for _, event := range result.Events {
		if event.TimestampBasis != "" {
			t.Fatalf("event timestamp must keep the default basis, got %q", event.TimestampBasis)
		}
	}
}

func TestParseCursorArtifactExcludesSystemToolsSkillsAndReasoning(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeCursorArtifact(t, []string{
		`{"role":"system","message":{"content":[{"type":"text","text":"EXCLUDED system api_key=private"}]}}`,
		`{"role":"tool","message":{"content":[{"type":"text","text":"EXCLUDED tool output"}]}}`,
		`{"type":"tool_call","role":"assistant","message":{"content":[{"type":"text","text":"EXCLUDED tool envelope"}]}}`,
		`{"role":"user","message":{"content":[{"type":"text","text":"<user_query>Human request</user_query>"},{"type":"tool_result","content":"EXCLUDED adjacent result"},{"type":"skill","text":"EXCLUDED skill"}]}}`,
		`{"role":"assistant","message":{"content":[{"type":"reasoning","text":"EXCLUDED reasoning"},{"type":"thinking","thinking":"EXCLUDED thinking"},{"type":"tool_use","name":"Shell","input":{"command":"EXCLUDED command"}},{"type":"text","text":"Assistant answer"}]}}`,
	}, day)

	result := parseCursorArtifact(path, day)
	assertCleanCursorResult(t, result)
	texts := cursorEventTexts(result.Events)
	if !strings.Contains(texts, "Human request") {
		t.Fatalf("expected wrapped human request in events: %q", texts)
	}
	for _, excluded := range []string{
		"EXCLUDED system", "private", "EXCLUDED tool output", "EXCLUDED tool envelope",
		"EXCLUDED adjacent result", "EXCLUDED skill", "EXCLUDED reasoning", "EXCLUDED thinking", "EXCLUDED command", "Assistant answer",
	} {
		if strings.Contains(texts, excluded) {
			t.Fatalf("did not expect %q in events: %q", excluded, texts)
		}
	}
}

func TestParseCursorArtifactReportsMalformedAndUnsupportedChatShapes(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeCursorArtifact(t, []string{
		`{"role":"user","message":{"content":[{"type":"text","text":"<user_query>valid request</user_query>"}]}}`,
		`{"role":"user","timestamp":"not-a-timestamp","message":{"content":[{"type":"text","text":"<user_query>invalid timestamp</user_query>"}]}}`,
		`{"role":"user","message":{"role":"assistant","content":[{"type":"text","text":"<user_query>mismatched role</user_query>"}]}}`,
		`{"role":"user","message":{"content":{"type":"text","text":"<user_query>unsupported object</user_query>"}}}`,
		`{"role":"user","message":{"content":[{"type":"text","text":"bare user text"}]}}`,
		`{"role":"user","message":{"content":[{"type":"text","text":"<user_query>first query</user_query>"},{"type":"text","text":"<user_query>second query</user_query>"}]}}`,
		`{"role":"assistant","message":`,
	}, day)

	result := parseCursorArtifact(path, day)
	if !result.ParseError {
		t.Fatalf("malformed JSONL must set ParseError: %#v", result)
	}
	if !result.UnsupportedSchema {
		t.Fatalf("invalid Cursor records must set UnsupportedSchema: %#v", result)
	}
	if result.Unreadable || result.UnsupportedStorage {
		t.Fatalf("unexpected storage flags: %#v", result)
	}
	if texts := cursorEventTexts(result.Events); texts != "valid request" {
		t.Fatalf("unsupported records must not leak raw payloads, got %q", texts)
	}
}

func TestUnwrapCursorUserQueryRequiresOneCompleteWrapper(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
		ok   bool
	}{
		{name: "exact wrapper", text: "  <user_query>\nrequest\n</user_query>  ", want: "request", ok: true},
		{name: "trailing content", text: "<user_query>request</user_query> trailing"},
		{name: "nested wrapper", text: "<user_query><user_query>request</user_query></user_query>"},
		{name: "empty wrapper", text: "<user_query> </user_query>"},
		{name: "plain text", text: " request "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := unwrapCursorUserQuery(test.text)
			if got != test.want || ok != test.ok {
				t.Fatalf("unwrapCursorUserQuery(%q) = (%q, %t), want (%q, %t)", test.text, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestParseCursorArtifactReportsUnreadableFile(t *testing.T) {
	result := parseCursorArtifact(filepath.Join(t.TempDir(), "missing.jsonl"), time.Now())
	if !result.Unreadable || len(result.Events) != 0 {
		t.Fatalf("missing artifact must be unreadable without events: %#v", result)
	}
}

func writeCursorArtifact(t *testing.T, lines []string, modifiedAt time.Time) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modifiedAt, modifiedAt); err != nil {
		t.Fatal(err)
	}
	return path
}

func cursorEventTexts(events []sourceEvent) string {
	texts := make([]string, 0, len(events))
	for _, event := range events {
		texts = append(texts, event.Text)
	}
	return strings.Join(texts, "\n")
}

func assertCleanCursorResult(t *testing.T, result artifactResult) {
	t.Helper()
	if result.ParseError || result.Truncated || result.Unreadable || result.UnsupportedSchema || result.UnsupportedStorage {
		t.Fatalf("unexpected parse flags: %#v", result)
	}
}
