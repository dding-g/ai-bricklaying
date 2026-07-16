package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMatchCodexArtifactScopesRolloutTranscripts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	nested := filepath.Join(root, "2026", "07", "16")

	tests := []struct {
		name   string
		path   string
		direct bool
		want   bool
	}{
		{name: "nested rollout", path: filepath.Join(nested, "rollout-2026-07-16-session.jsonl"), want: true},
		{name: "root rollout", path: filepath.Join(root, "rollout-session.jsonl"), want: true},
		{name: "cumulative history", path: filepath.Join(root, "history.jsonl"), want: false},
		{name: "arbitrary jsonl", path: filepath.Join(nested, "events.jsonl"), want: false},
		{name: "compressed rollout", path: filepath.Join(nested, "rollout-session.jsonl.gz"), want: false},
		{name: "outside root", path: filepath.Join(filepath.Dir(root), "rollout-outside.jsonl"), want: false},
		{name: "direct arbitrary jsonl", path: filepath.Join(root, "chosen.jsonl"), direct: true, want: true},
		{name: "direct non-jsonl", path: filepath.Join(root, "chosen.json"), direct: true, want: false},
		{name: "direct path mismatch", path: filepath.Join(root, "other.jsonl"), direct: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matchRoot := root
			if test.direct {
				matchRoot = filepath.Join(root, "chosen.jsonl")
			}
			if got := matchCodexArtifact(matchRoot, test.path, test.direct); got != test.want {
				t.Fatalf("matchCodexArtifact(%q, %q, %t) = %t, want %t", matchRoot, test.path, test.direct, got, test.want)
			}
		})
	}
}

func TestParseCodexArtifactUsesItemCompletedTimestampAndRequestedTimezone(t *testing.T) {
	seoul := time.FixedZone("Asia/Seoul", 9*60*60)
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, seoul)
	path := writeCodexArtifact(t, []string{
		`{"timestamp":"2026-07-15T14:59:59Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"u0","content":[{"type":"text","text":"previous day"}]}}}`,
		`{"timestamp":"2026-07-15T15:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"u1","content":[{"type":"text","text":"start boundary"},{"type":"skill","name":"private skill","path":"/private"}]}}}`,
		`{"timestamp":"2026-07-16T03:04:05.123Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"a1","content":[{"type":"Text","text":"implemented adapter"}],"phase":"final_answer"}}}`,
		`{"timestamp":"2026-07-16T14:59:59Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"u2","content":[{"type":"text","text":"end boundary"},{"type":"image","image_url":"private image"}]}}}`,
		`{"timestamp":"2026-07-16T15:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"a2","content":[{"type":"Text","text":"next day"}]}}}`,
		`{"timestamp":"2026-07-16T10:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"Reasoning","id":"r1","raw_content":["EXCLUDED reasoning"]}}}`,
		`{"timestamp":"2026-07-16T11:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"c1","aggregated_output":"EXCLUDED tool output"}}}`,
	})

	result := parseCodexArtifact(path, day)
	if result.ParseError || result.Truncated || result.Unreadable || result.UnsupportedSchema || result.UnsupportedStorage {
		t.Fatalf("unexpected parse flags: %#v", result)
	}
	texts := codexEventTexts(result.Events)
	for _, expected := range []string{"start boundary", "implemented adapter", "end boundary"} {
		if !strings.Contains(texts, expected) {
			t.Fatalf("expected %q in events: %q", expected, texts)
		}
	}
	for _, excluded := range []string{"previous day", "next day", "private skill", "/private", "private image", "EXCLUDED reasoning", "EXCLUDED tool output"} {
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

func TestParseCodexArtifactPrefersCurrentLifecycleWithoutMixing(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeCodexArtifact(t, []string{
		`{"timestamp":"2026-07-16T01:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"EXCLUDED response request"}]}}`,
		`{"timestamp":"2026-07-16T01:01:00Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"EXCLUDED response answer"}]}}`,
		`{"timestamp":"2026-07-16T01:02:00Z","type":"event_msg","payload":{"type":"user_message","message":"EXCLUDED direct request"}}`,
		`{"timestamp":"2026-07-16T01:03:00Z","type":"event_msg","payload":{"type":"agent_message","message":"EXCLUDED direct answer"}}`,
		`{"timestamp":"2026-07-16T01:04:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"u1","content":[{"type":"text","text":"canonical request"}]}}}`,
		`{"timestamp":"2026-07-16T01:05:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"a1","content":[{"type":"Text","text":"canonical answer"}]}}}`,
	})

	result := parseCodexArtifact(path, day)
	if result.ParseError || result.UnsupportedSchema || result.Unreadable {
		t.Fatalf("unexpected parse flags: %#v", result)
	}
	texts := codexEventTexts(result.Events)
	if texts != "canonical request\ncanonical answer" {
		t.Fatalf("expected only item_completed lifecycle text, got %q", texts)
	}
}

func TestParseCodexArtifactPrefersDirectLifecycleOverResponseFallback(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeCodexArtifact(t, []string{
		`{"timestamp":"2026-07-16T01:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"EXCLUDED fallback request"}]}}`,
		`{"timestamp":"2026-07-16T02:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"direct request"}}`,
		`{"timestamp":"2026-07-16T03:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"direct answer"}}`,
		`{"timestamp":"2026-07-16T04:00:00Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"EXCLUDED reasoning"}}`,
		`{"timestamp":"2026-07-16T05:00:00Z","type":"event_msg","payload":{"type":"exec_command_end","aggregated_output":"EXCLUDED command output"}}`,
	})

	result := parseCodexArtifact(path, day)
	if result.ParseError || result.UnsupportedSchema || result.Unreadable {
		t.Fatalf("unexpected parse flags: %#v", result)
	}
	if texts := codexEventTexts(result.Events); texts != "direct request\ndirect answer" {
		t.Fatalf("expected only direct lifecycle text, got %q", texts)
	}
}

func TestParseCodexArtifactDoesNotLetPriorDayHigherTierShadowToday(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeCodexArtifact(t, []string{
		`{"timestamp":"2026-07-15T23:59:59Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"u1","content":[{"type":"text","text":"EXCLUDED prior-day canonical request"}]}}}`,
		`{"timestamp":"2026-07-16T01:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"today direct request"}}`,
		`{"timestamp":"2026-07-16T02:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"today direct answer"}}`,
		`{"timestamp":"2026-07-16T03:00:00Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"EXCLUDED today fallback"}]}}`,
	})

	result := parseCodexArtifact(path, day)
	if result.ParseError || result.UnsupportedSchema || result.Unreadable {
		t.Fatalf("unexpected parse flags: %#v", result)
	}
	if texts := codexEventTexts(result.Events); texts != "today direct request\ntoday direct answer" {
		t.Fatalf("expected today's direct lifecycle tier only, got %q", texts)
	}
}

func TestParseCodexArtifactKeepsMalformedHigherTierSignal(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeCodexArtifact(t, []string{
		`{"timestamp":"not-a-timestamp","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"u1","content":[{"type":"text","text":"unverifiable canonical request"}]}}}`,
		`{"timestamp":"2026-07-16T01:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"verified direct request"}}`,
	})

	result := parseCodexArtifact(path, day)
	if !result.UnsupportedSchema {
		t.Fatalf("malformed higher tier must remain visible: %#v", result)
	}
	if result.ParseError || result.Unreadable {
		t.Fatalf("unexpected read flags: %#v", result)
	}
	if texts := codexEventTexts(result.Events); texts != "verified direct request" {
		t.Fatalf("expected verified lower-tier event without mixing, got %q", texts)
	}
}

func TestParseCodexArtifactFallbackExcludesAutomaticContextAndPrivilegedRecords(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeCodexArtifact(t, []string{
		`{"timestamp":"2026-07-16T00:00:00Z","type":"session_meta","payload":{"base_instructions":"EXCLUDED base instructions","cwd":"/private"}}`,
		`{"timestamp":"2026-07-16T00:01:00Z","type":"turn_context","payload":{"developer_instructions":"EXCLUDED developer context"}}`,
		`{"timestamp":"2026-07-16T00:02:00Z","type":"world_state","payload":{"state":"EXCLUDED world state"}}`,
		`{"timestamp":"2026-07-16T00:03:00Z","type":"response_item","payload":{"type":"message","role":"system","content":[{"type":"input_text","text":"EXCLUDED system"}]}}`,
		`{"timestamp":"2026-07-16T00:04:00Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"EXCLUDED developer"}]}}`,
		`{"timestamp":"2026-07-16T00:05:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions for /private\n<INSTRUCTIONS>EXCLUDED AGENTS</INSTRUCTIONS>"}]}}`,
		`{"timestamp":"2026-07-16T00:06:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<recommended_plugins>EXCLUDED host recommendations</recommended_plugins>"}]}}`,
		`{"timestamp":"2026-07-16T01:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"actual request\n\n<environment_context>EXCLUDED environment</environment_context>"},{"type":"input_image","image_url":"EXCLUDED image"}]}}`,
		`{"timestamp":"2026-07-16T02:00:00Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"assistant answer"},{"type":"reasoning","text":"EXCLUDED private reasoning"}]}}`,
		`{"timestamp":"2026-07-16T03:00:00Z","type":"response_item","payload":{"type":"function_call_output","output":"EXCLUDED tool output"}}`,
	})

	result := parseCodexArtifact(path, day)
	if result.ParseError || result.UnsupportedSchema || result.Unreadable {
		t.Fatalf("unexpected parse flags: %#v", result)
	}
	texts := codexEventTexts(result.Events)
	if texts != "actual request\nassistant answer" {
		t.Fatalf("unexpected fallback text: %q", texts)
	}
	for _, excluded := range []string{
		"EXCLUDED base", "EXCLUDED developer", "EXCLUDED world", "EXCLUDED system", "EXCLUDED AGENTS",
		"EXCLUDED host recommendations", "EXCLUDED environment", "EXCLUDED image", "EXCLUDED private reasoning", "EXCLUDED tool output", "/private",
	} {
		if strings.Contains(texts, excluded) {
			t.Fatalf("did not expect %q in events: %q", excluded, texts)
		}
	}
}

func TestParseCodexArtifactReportsMalformedSelectedLifecycle(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	path := writeCodexArtifact(t, []string{
		`{"timestamp":"2026-07-16T01:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"u1","content":[{"type":"text","text":"valid request"}]}}}`,
		`{"timestamp":"not-a-timestamp","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"a1","content":[{"type":"Text","text":"invalid timestamp"}]}}}`,
		`{"timestamp":"2026-07-16T03:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"a2","content":{"type":"Text","text":"unsupported content"}}}}`,
		`{"timestamp":"2026-07-16T04:00:00Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"EXCLUDED fallback"}]}}`,
		`{"timestamp":"2026-07-16T05:00:00Z","type":"event_msg",`,
	})

	result := parseCodexArtifact(path, day)
	if !result.ParseError {
		t.Fatalf("malformed JSONL must set ParseError: %#v", result)
	}
	if !result.UnsupportedSchema {
		t.Fatalf("invalid selected lifecycle records must set UnsupportedSchema: %#v", result)
	}
	if result.Unreadable || result.UnsupportedStorage {
		t.Fatalf("unexpected storage flags: %#v", result)
	}
	if texts := codexEventTexts(result.Events); texts != "valid request" {
		t.Fatalf("unsupported records and fallback must not leak, got %q", texts)
	}
}

func TestParseCodexArtifactReportsUnreadableFile(t *testing.T) {
	result := parseCodexArtifact(filepath.Join(t.TempDir(), "missing.jsonl"), time.Now())
	if !result.Unreadable || len(result.Events) != 0 {
		t.Fatalf("missing artifact must be unreadable without events: %#v", result)
	}
}

func writeCodexArtifact(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rollout-session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func codexEventTexts(events []sourceEvent) string {
	texts := make([]string, 0, len(events))
	for _, event := range events {
		texts = append(texts, event.Text)
	}
	return strings.Join(texts, "\n")
}
