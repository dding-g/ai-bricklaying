package slack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildPayloadUsesSavedMarkdownContract(t *testing.T) {
	markdown := `# AI Bricklaying Daily Summary - 2026-05-19

## Today's Takeaways
- First lesson stays first.
- Second lesson stays second.

## Better AI Usage Next Time
- Ask for verification evidence.
`

	payload := BuildPayload(markdown)

	if payload.Text != "AI Bricklaying Daily Summary - 2026-05-19" {
		t.Fatalf("text = %q", payload.Text)
	}
	if len(payload.Blocks) == 0 || len(payload.Messages) != 1 {
		t.Fatalf("payload should expose first batch and one message: %#v", payload)
	}
	if !reflect.DeepEqual(payload.Blocks, payload.Messages[0].Blocks) {
		t.Fatalf("top-level blocks should mirror first message")
	}
	wantSections := []string{"Today's Takeaways", "Better AI Usage Next Time"}
	if !reflect.DeepEqual(payload.Verification.TopLevelSections, wantSections) {
		t.Fatalf("top sections = %#v, want %#v", payload.Verification.TopLevelSections, wantSections)
	}
	if !payload.Verification.AllTopLevelSectionsCovered {
		t.Fatalf("expected all sections covered: %#v", payload.Verification)
	}
	combined := blockText(payload.Messages)
	assertInOrder(t, combined, []string{"Today's Takeaways", "First lesson stays first.", "Second lesson stays second.", "Better AI Usage Next Time", "Ask for verification evidence."})
}

func TestBuildPayloadSplitsLongMarkdownAndRedactsSecrets(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "tests", "contracts", "fixtures", "slack-long-summary.ko.md")
	markdownBytes, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	markdown := string(markdownBytes)
	if !strings.Contains(markdown, "fixture-secret") || !strings.Contains(markdown, "fixturetoken123456") {
		t.Fatalf("fixture should include fake secrets")
	}

	payload := BuildPayload(markdown)
	sections := TopLevelSections(markdown)

	if len(sections) <= 50 {
		t.Fatalf("fixture should exercise long section coverage")
	}
	if len(payload.Messages) <= 1 {
		t.Fatalf("long fixture should split into multiple messages")
	}
	if !strings.HasSuffix(payload.Messages[0].Text, "(1/"+itoa(len(payload.Messages))+")") {
		t.Fatalf("first message text missing split suffix: %q", payload.Messages[0].Text)
	}
	if !strings.HasSuffix(payload.Messages[len(payload.Messages)-1].Text, "("+itoa(len(payload.Messages))+"/"+itoa(len(payload.Messages))+")") {
		t.Fatalf("last message text missing split suffix: %q", payload.Messages[len(payload.Messages)-1].Text)
	}
	if !reflect.DeepEqual(payload.Verification.TopLevelSections, sections) {
		t.Fatalf("top sections = %#v, want %#v", payload.Verification.TopLevelSections, sections)
	}
	if !reflect.DeepEqual(payload.Verification.CoveredTopLevelSections, sections) {
		t.Fatalf("covered sections = %#v, want %#v", payload.Verification.CoveredTopLevelSections, sections)
	}
	for index, message := range payload.Messages {
		if len(message.Blocks) > maxBlocksPerMessage {
			t.Fatalf("message %d has %d blocks, want <= %d", index, len(message.Blocks), maxBlocksPerMessage)
		}
	}
	combined := blockText(payload.Messages)
	assertInOrder(t, combined, []string{"섹션 01 계약 검증", "항목 01", "섹션 02 계약 검증", "항목 02", "섹션 60 계약 검증", "항목 60", "비밀 값 검증"})
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"fixture-secret", "fixturetoken123456", "https://hooks.slack.com/services/T000/B000"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("payload leaked %q: %s", forbidden, string(encoded))
		}
	}
}

func assertInOrder(t *testing.T, haystack string, needles []string) {
	t.Helper()
	cursor := -1
	for _, needle := range needles {
		next := strings.Index(haystack[cursor+1:], needle)
		if next < 0 {
			t.Fatalf("missing %q in %q", needle, haystack)
		}
		cursor += next + 1
	}
}
