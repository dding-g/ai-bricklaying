package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMatchClaudeArtifactScopesMainProjectTranscripts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	project := filepath.Join(root, "-Users-example-project")

	tests := []struct {
		name   string
		path   string
		direct bool
		want   bool
	}{
		{name: "main transcript", path: filepath.Join(project, "session.jsonl"), want: true},
		{name: "different extension", path: filepath.Join(project, "session.json"), want: false},
		{name: "project directory itself", path: project, want: false},
		{name: "nested transcript", path: filepath.Join(project, "nested", "session.jsonl"), want: false},
		{name: "subagent transcript", path: filepath.Join(project, "session", "subagents", "agent.jsonl"), want: false},
		{name: "tool result", path: filepath.Join(project, "tool-results", "result.jsonl"), want: false},
		{name: "memory", path: filepath.Join(project, "memory", "note.jsonl"), want: false},
		{name: "reserved root project name", path: filepath.Join(root, "memory", "note.jsonl"), want: false},
		{name: "outside root", path: filepath.Join(filepath.Dir(root), "outside", "session.jsonl"), want: false},
		{name: "direct jsonl", path: filepath.Join(root, "session.jsonl"), direct: true, want: true},
		{name: "direct non-jsonl", path: filepath.Join(root, "session.json"), direct: true, want: false},
		{name: "direct path mismatch", path: filepath.Join(root, "other.jsonl"), direct: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matchRoot := root
			if test.direct {
				matchRoot = filepath.Join(root, "session.jsonl")
			}
			if got := matchClaudeArtifact(matchRoot, test.path, test.direct); got != test.want {
				t.Fatalf("matchClaudeArtifact(%q, %q, %t) = %t, want %t", matchRoot, test.path, test.direct, got, test.want)
			}
		})
	}
}

func TestParseClaudeArtifactUsesEventTimestampAndRequestedTimezone(t *testing.T) {
	seoul := time.FixedZone("Asia/Seoul", 9*60*60)
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, seoul)
	path := writeClaudeArtifact(t, []string{
		`{"type":"user","timestamp":"2026-07-15T14:59:59Z","message":{"role":"user","content":"previous day"}}`,
		`{"type":"user","timestamp":"2026-07-15T15:00:00Z","message":{"role":"user","content":"start boundary"}}`,
		`{"type":"assistant","timestamp":"2026-07-16T03:04:05.123Z","message":{"type":"message","role":"assistant","content":[{"type":"text","text":"implemented adapter"},{"type":"tool_use","name":"Read","input":{"file_path":"private"}}]}}`,
		`{"type":"user","timestamp":"2026-07-16T14:59:59Z","message":{"role":"user","content":[{"type":"text","text":"end boundary"},{"type":"image","source":{"data":"private image"}}]}}`,
		`{"type":"assistant","timestamp":"2026-07-16T15:00:00Z","message":{"role":"assistant","content":"next day"}}`,
	})

	result := parseClaudeArtifact(path, day)
	if result.ParseError || result.Truncated || result.Unreadable || result.UnsupportedSchema || result.UnsupportedStorage {
		t.Fatalf("unexpected parse flags: %#v", result)
	}

	texts := claudeEventTexts(result.Events)
	for _, expected := range []string{"start boundary", "implemented adapter", "end boundary"} {
		if !strings.Contains(texts, expected) {
			t.Fatalf("expected %q in events: %q", expected, texts)
		}
	}
	for _, excluded := range []string{"previous day", "next day", "private", "private image"} {
		if strings.Contains(texts, excluded) {
			t.Fatalf("did not expect %q in events: %q", excluded, texts)
		}
	}
	for index := 1; index < len(result.Events); index++ {
		if result.Events[index].Sequence <= result.Events[index-1].Sequence {
			t.Fatalf("event sequence must preserve JSONL order: %#v", result.Events)
		}
	}
}

func TestParseClaudeArtifactExcludesMetadataSidechainsSummariesToolsAndErrors(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeClaudeArtifact(t, []string{
		`{"type":"user","timestamp":"2026-07-16T01:00:00Z","isMeta":true,"message":{"role":"user","content":"EXCLUDED meta"}}`,
		`{"type":"assistant","timestamp":"2026-07-16T02:00:00Z","isSidechain":true,"message":{"role":"assistant","content":"EXCLUDED sidechain"}}`,
		`{"type":"user","timestamp":"2026-07-16T03:00:00Z","isCompactSummary":true,"message":{"role":"user","content":"EXCLUDED compact summary"}}`,
		`{"type":"user","timestamp":"2026-07-16T03:30:00Z","message":{"role":"user","isCompactSummary":true,"content":"EXCLUDED nested compact summary"}}`,
		`{"type":"summary","timestamp":"2026-07-16T04:00:00Z","summary":"EXCLUDED summary"}`,
		`{"type":"system","timestamp":"2026-07-16T05:00:00Z","message":{"role":"system","content":"EXCLUDED system api_key=private"}}`,
		`{"type":"user","timestamp":"2026-07-16T06:00:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":"EXCLUDED tool result"}]},"toolUseResult":{"stdout":"EXCLUDED stdout"}}`,
		`{"type":"assistant","timestamp":"2026-07-16T07:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"EXCLUDED command"}},{"type":"error","text":"EXCLUDED block error"}]}}`,
		`{"type":"assistant","timestamp":"2026-07-16T08:00:00Z","error":"EXCLUDED record error","message":{"role":"assistant","content":"EXCLUDED failed assistant text"}}`,
		`{"type":"user","timestamp":"2026-07-16T09:00:00Z","message":{"role":"user","content":[{"type":"text","text":"human request"},{"type":"tool_result","content":"EXCLUDED adjacent result"}]}}`,
		`{"type":"assistant","timestamp":"2026-07-16T10:00:00Z","message":{"role":"assistant","content":[{"type":"text","text":"assistant answer"},{"type":"thinking","thinking":"EXCLUDED thinking"}]}}`,
	})

	result := parseClaudeArtifact(path, day)
	if result.ParseError || result.Truncated || result.Unreadable || result.UnsupportedStorage {
		t.Fatalf("unexpected parse flags: %#v", result)
	}
	texts := claudeEventTexts(result.Events)
	for _, expected := range []string{"human request", "assistant answer"} {
		if !strings.Contains(texts, expected) {
			t.Fatalf("expected %q in events: %q", expected, texts)
		}
	}
	for _, excluded := range []string{
		"EXCLUDED meta", "EXCLUDED sidechain", "EXCLUDED compact summary", "EXCLUDED nested compact summary",
		"EXCLUDED summary", "EXCLUDED system", "EXCLUDED tool result", "EXCLUDED stdout",
		"EXCLUDED command", "EXCLUDED block error", "EXCLUDED record error", "EXCLUDED failed assistant text",
		"EXCLUDED adjacent result", "EXCLUDED thinking", "private",
	} {
		if strings.Contains(texts, excluded) {
			t.Fatalf("did not expect %q in events: %q", excluded, texts)
		}
	}
}

func TestParseClaudeArtifactReportsMalformedAndUnsupportedChatShapes(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeClaudeArtifact(t, []string{
		`{"type":"user","timestamp":"2026-07-16T01:00:00Z","message":{"role":"user","content":"valid request"}}`,
		`{"type":"user","timestamp":"not-a-timestamp","message":{"role":"user","content":"invalid timestamp"}}`,
		`{"type":"assistant","timestamp":"2026-07-16T02:00:00Z","message":{"role":"user","content":"mismatched role"}}`,
		`{"type":"assistant","timestamp":"2026-07-16T03:00:00Z","message":{"role":"assistant","content":{"text":"unsupported content object"}}}`,
		`{"type":"assistant","timestamp":`,
	})

	result := parseClaudeArtifact(path, day)
	if !result.ParseError {
		t.Fatalf("malformed JSONL must set ParseError: %#v", result)
	}
	if !result.UnsupportedSchema {
		t.Fatalf("invalid chat records must set UnsupportedSchema: %#v", result)
	}
	if result.Unreadable || result.UnsupportedStorage {
		t.Fatalf("unexpected storage flags: %#v", result)
	}
	texts := claudeEventTexts(result.Events)
	if texts != "valid request" {
		t.Fatalf("unsupported records must not leak raw payloads, got %q", texts)
	}
}

func TestParseClaudeArtifactReportsUnreadableFile(t *testing.T) {
	result := parseClaudeArtifact(filepath.Join(t.TempDir(), "missing.jsonl"), time.Now())
	if !result.Unreadable || len(result.Events) != 0 {
		t.Fatalf("missing artifact must be unreadable without events: %#v", result)
	}
}

func writeClaudeArtifact(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func claudeEventTexts(events []sourceEvent) string {
	texts := make([]string, 0, len(events))
	for _, event := range events {
		texts = append(texts, event.Text)
	}
	return strings.Join(texts, "\n")
}
