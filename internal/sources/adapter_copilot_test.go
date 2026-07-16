package sources

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMatchCopilotArtifactScopesWorkspaceChatSessions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaceStorage")
	valid := filepath.Join(root, "1234abcd", "chatSessions", "session.jsonl")
	tests := []struct {
		name   string
		path   string
		direct bool
		want   bool
	}{
		{name: "workspace chat session", path: valid, want: true},
		{name: "legacy json", path: filepath.Join(root, "1234abcd", "chatSessions", "session.json"), want: false},
		{name: "empty session name", path: filepath.Join(root, "1234abcd", "chatSessions", ".jsonl"), want: false},
		{name: "workspace metadata", path: filepath.Join(root, "1234abcd", "workspace.jsonl"), want: false},
		{name: "chat editing session", path: filepath.Join(root, "1234abcd", "chatEditingSessions", "session.jsonl"), want: false},
		{name: "nested chat artifact", path: filepath.Join(root, "1234abcd", "chatSessions", "nested", "session.jsonl"), want: false},
		{name: "global storage chat", path: filepath.Join(root, "globalStorage", "github.copilot-chat", "session.jsonl"), want: false},
		{name: "outside root", path: filepath.Join(filepath.Dir(root), "outside", "chatSessions", "session.jsonl"), want: false},
		{name: "direct jsonl", path: filepath.Join(root, "copilot.jsonl"), direct: true, want: true},
		{name: "direct non-jsonl", path: filepath.Join(root, "copilot.json"), direct: true, want: false},
		{name: "direct path mismatch", path: filepath.Join(root, "other.jsonl"), direct: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matchRoot := root
			if test.direct {
				matchRoot = filepath.Join(root, "copilot"+filepath.Ext(test.path))
			}
			if got := matchCopilotArtifact(matchRoot, test.path, test.direct); got != test.want {
				t.Fatalf("matchCopilotArtifact(%q, %q, %t) = %t, want %t", matchRoot, test.path, test.direct, got, test.want)
			}
		})
	}
}

func TestParseCopilotArtifactReplaysMutationLogAndUsesEventTime(t *testing.T) {
	seoul := time.FixedZone("Asia/Seoul", 9*60*60)
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, seoul)
	creation := time.Date(2026, 7, 16, 0, 0, 0, 0, seoul).UnixMilli()
	start := time.Date(2026, 7, 16, 0, 0, 0, 0, seoul).UnixMilli()
	end := time.Date(2026, 7, 16, 23, 59, 59, 0, seoul).UnixMilli()
	previous := time.Date(2026, 7, 15, 23, 59, 59, 0, seoul).UnixMilli()
	next := time.Date(2026, 7, 17, 0, 0, 0, 0, seoul).UnixMilli()

	path := writeCopilotArtifact(t, []string{
		fmt.Sprintf(`{"kind":0,"v":{"version":3,"creationDate":%d,"sessionId":"session-1","requests":[{"requestId":"r0","timestamp":%d,"agent":{"extensionId":{"value":"GitHub.copilot-chat","_lower":"github.copilot-chat"}},"message":{"text":"previous day","parts":[]},"response":[{"value":"previous response"}],"responseTimestamp":%d}]}}`, creation, previous, previous),
		fmt.Sprintf(`{"kind":2,"k":["requests"],"v":[{"requestId":"r1","timestamp":%d,"agent":{"extensionId":{"value":"GitHub.copilot-chat","_lower":"github.copilot-chat"}},"message":{"text":"start boundary","parts":[]},"response":[]}]}`, start),
		`{"kind":1,"k":["requests",1,"response"],"v":[{"value":"implemented adapter"},{"kind":"toolInvocationSerialized","toolSpecificData":"EXCLUDED tool payload"}]}`,
		fmt.Sprintf(`{"kind":1,"k":["requests",1,"responseTimestamp"],"v":%d}`, end),
		`{"kind":2,"k":["requests",1,"response"],"v":[{"value":"verified output"}]}`,
		`{"kind":3,"k":["requests",1,"message","parts"]}`,
		fmt.Sprintf(`{"kind":2,"k":["requests"],"v":[{"requestId":"r2","timestamp":%d,"agent":{"extensionId":{"value":"GitHub.copilot-chat","_lower":"github.copilot-chat"}},"message":{"text":"next day","parts":[]},"response":[{"value":"next response"}],"responseTimestamp":%d}]}`, next, next),
	})

	result := parseCopilotArtifact(path, day)
	if result.ParseError || result.Truncated || result.Unreadable || result.UnsupportedSchema || result.UnsupportedStorage {
		t.Fatalf("unexpected parse flags: %#v", result)
	}
	texts := copilotEventTexts(result.Events)
	for _, expected := range []string{"start boundary", "implemented adapter", "verified output"} {
		if !strings.Contains(texts, expected) {
			t.Fatalf("expected %q in events: %q", expected, texts)
		}
	}
	for _, excluded := range []string{"previous day", "previous response", "next day", "next response", "EXCLUDED tool payload"} {
		if strings.Contains(texts, excluded) {
			t.Fatalf("did not expect %q in events: %q", excluded, texts)
		}
	}
	for index := 1; index < len(result.Events); index++ {
		if result.Events[index].Sequence <= result.Events[index-1].Sequence {
			t.Fatalf("event sequence must preserve request/response order: %#v", result.Events)
		}
	}
}

func TestParseCopilotArtifactExcludesSystemToolsThinkingAndPrivateMetadata(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	timestamp := time.Date(2026, 7, 16, 1, 2, 3, 0, time.UTC).UnixMilli()
	path := writeCopilotArtifact(t, []string{
		fmt.Sprintf(`{"kind":0,"v":{"version":3,"creationDate":%d,"sessionId":"session-privacy","responderUsername":"EXCLUDED identity","repoData":{"remoteUrl":"EXCLUDED repo"},"requests":[{"requestId":"system","timestamp":%d,"agent":{"extensionId":{"value":"GitHub.copilot-chat","_lower":"github.copilot-chat"}},"isSystemInitiated":true,"message":{"text":"EXCLUDED system request","parts":[]},"response":[{"value":"EXCLUDED system response"}]},{"requestId":"human","timestamp":%d,"agent":{"extensionId":{"value":"GitHub.copilot-chat","_lower":"github.copilot-chat"}},"message":{"text":"human request","parts":[],"private":"EXCLUDED message metadata"},"variableData":{"variables":[{"value":"EXCLUDED attachment"}]},"result":{"metadata":"EXCLUDED result"},"response":[{"value":"assistant answer","isTrusted":true},{"kind":"toolInvocationSerialized","invocationMessage":"EXCLUDED tool"},{"kind":"thinking","value":"EXCLUDED thinking"},{"kind":"command","command":{"title":"EXCLUDED command"}},{"kind":"progressMessage","content":"EXCLUDED progress"}],"responseTimestamp":%d}]}}`, timestamp, timestamp, timestamp, timestamp),
	})

	result := parseCopilotArtifact(path, day)
	if result.ParseError || result.Truncated || result.Unreadable || result.UnsupportedSchema || result.UnsupportedStorage {
		t.Fatalf("unexpected parse flags: %#v", result)
	}
	texts := copilotEventTexts(result.Events)
	for _, expected := range []string{"human request", "assistant answer"} {
		if !strings.Contains(texts, expected) {
			t.Fatalf("expected %q in events: %q", expected, texts)
		}
	}
	for _, excluded := range []string{"EXCLUDED identity", "EXCLUDED repo", "EXCLUDED system", "EXCLUDED message metadata", "EXCLUDED attachment", "EXCLUDED result", "EXCLUDED tool", "EXCLUDED thinking", "EXCLUDED command", "EXCLUDED progress"} {
		if strings.Contains(texts, excluded) {
			t.Fatalf("did not expect %q in events: %q", excluded, texts)
		}
	}
}

func TestParseCopilotArtifactFallsBackToSessionCreationDate(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	creation := time.Date(2026, 7, 16, 2, 3, 4, 0, time.UTC).UnixMilli()
	path := writeCopilotArtifact(t, []string{
		fmt.Sprintf(`{"kind":0,"v":{"version":3,"creationDate":%d,"requests":[{"requestId":"r1","agent":{"extensionId":{"value":"GitHub.copilot-chat","_lower":"github.copilot-chat"}},"message":{"text":"fallback request","parts":[]},"response":[{"value":"fallback response"}]}]}}`, creation),
	})

	result := parseCopilotArtifact(path, day)
	if result.ParseError || result.Unreadable || result.UnsupportedSchema {
		t.Fatalf("unexpected parse flags: %#v", result)
	}
	if texts := copilotEventTexts(result.Events); texts != "fallback request\nfallback response" {
		t.Fatalf("expected session timestamp fallback, got %q", texts)
	}
}

func TestParseCopilotArtifactReportsUnknownMutationAndResponseSchemas(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	timestamp := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC).UnixMilli()

	t.Run("unknown mutation aborts replay", func(t *testing.T) {
		path := writeCopilotArtifact(t, []string{
			fmt.Sprintf(`{"kind":0,"v":{"version":3,"creationDate":%d,"requests":[{"requestId":"r1","timestamp":%d,"agent":{"extensionId":{"value":"GitHub.copilot-chat","_lower":"github.copilot-chat"}},"message":{"text":"must not leak","parts":[]}}]}}`, timestamp, timestamp),
			`{"kind":9,"k":["requests"],"v":[{"private":"unknown"}]}`,
		})
		result := parseCopilotArtifact(path, day)
		if !result.UnsupportedSchema || len(result.Events) != 0 {
			t.Fatalf("unknown mutation must fail closed: %#v", result)
		}
	})

	t.Run("unknown response object is not exposed", func(t *testing.T) {
		path := writeCopilotArtifact(t, []string{
			fmt.Sprintf(`{"kind":0,"v":{"version":3,"creationDate":%d,"requests":[{"requestId":"r1","timestamp":%d,"agent":{"extensionId":{"value":"GitHub.copilot-chat","_lower":"github.copilot-chat"}},"message":{"text":"valid request","parts":[]},"response":[{"value":"EXCLUDED ambiguous","privateMetadata":"EXCLUDED secret"}],"responseTimestamp":%d}]}}`, timestamp, timestamp, timestamp),
		})
		result := parseCopilotArtifact(path, day)
		if !result.UnsupportedSchema {
			t.Fatalf("unknown response schema must be reported: %#v", result)
		}
		texts := copilotEventTexts(result.Events)
		if texts != "valid request" || strings.Contains(texts, "EXCLUDED") {
			t.Fatalf("ambiguous response must not be exposed, got %q", texts)
		}
	})

	t.Run("unknown session version", func(t *testing.T) {
		path := writeCopilotArtifact(t, []string{
			fmt.Sprintf(`{"kind":0,"v":{"version":4,"creationDate":%d,"requests":[]}}`, timestamp),
		})
		result := parseCopilotArtifact(path, day)
		if !result.UnsupportedSchema || len(result.Events) != 0 {
			t.Fatalf("unknown session version must be unsupported: %#v", result)
		}
	})
}

func TestParseCopilotArtifactRejectsOtherVSCodeChatAgents(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	timestamp := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC).UnixMilli()
	path := writeCopilotArtifact(t, []string{
		fmt.Sprintf(`{"kind":0,"v":{"version":3,"creationDate":%d,"requests":[{"requestId":"other","timestamp":%d,"agent":{"extensionId":{"value":"example.other-chat","_lower":"example.other-chat"}},"message":{"text":"EXCLUDED other agent request","parts":[]},"response":[{"value":"EXCLUDED other agent response"}],"responseTimestamp":%d},{"requestId":"wrong-case","timestamp":%d,"agent":{"extensionId":{"value":"github.copilot-chat","_lower":"github.copilot-chat"}},"message":{"text":"EXCLUDED case mismatch","parts":[]}},{"requestId":"missing","timestamp":%d,"message":{"text":"EXCLUDED unowned request","parts":[]}}]}}`, timestamp, timestamp, timestamp, timestamp, timestamp),
	})

	result := parseCopilotArtifact(path, day)
	if !result.UnsupportedSchema || len(result.Events) != 0 {
		t.Fatalf("non-Copilot VS Code agents must be unsupported without events: %#v", result)
	}
}

func TestParseCopilotArtifactOversizedTailWithoutInitialFailsClosed(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	timestamp := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC).UnixMilli()
	initial := fmt.Sprintf(`{"kind":0,"v":{"version":3,"creationDate":%d,"requests":[]}}`+"\n", timestamp)
	oversizedMutation := `{"kind":1,"k":["customTitle"],"v":"` + strings.Repeat("x", DefaultScanFileBytes+1024) + `"}` + "\n"
	path := filepath.Join(t.TempDir(), "oversized.jsonl")
	if err := os.WriteFile(path, []byte(initial+oversizedMutation), 0o600); err != nil {
		t.Fatal(err)
	}

	result := parseCopilotArtifact(path, day)
	if !result.Truncated || !result.UnsupportedSchema || len(result.Events) != 0 {
		t.Fatalf("tail without initial state must be truncated and unsupported: %#v", result)
	}
}

func TestParseCopilotArtifactMalformedMutationLogFailsClosed(t *testing.T) {
	day := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	timestamp := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC).UnixMilli()
	path := writeCopilotArtifact(t, []string{
		fmt.Sprintf(`{"kind":0,"v":{"version":3,"creationDate":%d,"requests":[{"requestId":"r1","timestamp":%d,"agent":{"extensionId":{"value":"GitHub.copilot-chat","_lower":"github.copilot-chat"}},"message":{"text":"must not survive malformed mutation","parts":[]}}]}}`, timestamp, timestamp),
		`{"kind":1,"k":["requests",0,"message","text"],"v":`,
	})

	result := parseCopilotArtifact(path, day)
	if !result.ParseError || len(result.Events) != 0 {
		t.Fatalf("malformed dependent log must fail closed: %#v", result)
	}
}

func TestParseCopilotArtifactReportsUnreadableFile(t *testing.T) {
	result := parseCopilotArtifact(filepath.Join(t.TempDir(), "missing.jsonl"), time.Now())
	if !result.Unreadable || len(result.Events) != 0 {
		t.Fatalf("missing artifact must be unreadable without events: %#v", result)
	}
}

func writeCopilotArtifact(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func copilotEventTexts(events []sourceEvent) string {
	texts := make([]string, 0, len(events))
	for _, event := range events {
		texts = append(texts, event.Text)
	}
	return strings.Join(texts, "\n")
}
