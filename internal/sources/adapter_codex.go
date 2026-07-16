package sources

import (
	"path/filepath"
	"strings"
	"time"
)

// matchCodexArtifact accepts Codex rollout JSONL files below the configured
// sessions root. A direct file override may use an arbitrary filename, but it
// must still identify that exact JSONL file.
func matchCodexArtifact(root string, path string, direct bool) bool {
	if !strings.EqualFold(filepath.Ext(path), ".jsonl") {
		return false
	}
	if direct {
		return candidateIdentity(root) == candidateIdentity(path)
	}

	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	return strings.HasPrefix(strings.ToLower(filepath.Base(path)), "rollout-")
}

type codexEventTier struct {
	events      []sourceEvent
	unsupported bool
}

// parseCodexArtifact extracts only explicit user and agent conversation
// messages. Codex rollouts can contain the same message in multiple protocol
// generations, so the parser selects exactly one tier for the requested day:
// current item_completed lifecycle items, direct legacy lifecycle events, or
// response_item as a final legacy fallback. A higher tier that has no verified
// event on that day does not hide relevant lower-tier activity.
func parseCodexArtifact(path string, day time.Time) artifactResult {
	lines, parseError, truncated, unreadable := readArtifactJSONLines(path)
	result := artifactResult{
		ParseError: parseError,
		Truncated:  truncated,
		Unreadable: unreadable,
	}

	var itemCompleted codexEventTier
	var directEvent codexEventTier
	var responseFallback codexEventTier
	globalUnsupported := false

	for _, line := range lines {
		event, ok := line.Value.(map[string]any)
		if !ok {
			globalUnsupported = true
			continue
		}

		switch strings.ToLower(normalizedText(event["type"])) {
		case "event_msg":
			payload, ok := event["payload"].(map[string]any)
			if !ok {
				globalUnsupported = true
				continue
			}
			parseCodexLifecycleEvent(event, payload, line.Sequence, day, &itemCompleted, &directEvent, &globalUnsupported)
		case "response_item":
			payload, ok := event["payload"].(map[string]any)
			if !ok {
				responseFallback.unsupported = true
				continue
			}
			parseCodexResponseFallback(event, payload, line.Sequence, day, &responseFallback)
		case "", "session_meta", "turn_context", "world_state", "compacted", "inter_agent_communication", "inter_agent_communication_metadata":
			// These records are metadata, model context, or synthetic state. They
			// are intentionally never conversation evidence.
		default:
			// Never recursively search a future outer schema. Surface the coverage
			// gap so it cannot be mistaken for verified no activity.
			globalUnsupported = true
		}
	}

	result.UnsupportedSchema = globalUnsupported
	switch {
	case len(itemCompleted.events) > 0:
		result.Events = itemCompleted.events
		result.UnsupportedSchema = result.UnsupportedSchema || itemCompleted.unsupported
	case len(directEvent.events) > 0:
		result.Events = directEvent.events
		result.UnsupportedSchema = result.UnsupportedSchema || itemCompleted.unsupported || directEvent.unsupported
	case len(responseFallback.events) > 0:
		result.Events = responseFallback.events
		result.UnsupportedSchema = result.UnsupportedSchema || itemCompleted.unsupported || directEvent.unsupported || responseFallback.unsupported
	default:
		result.UnsupportedSchema = result.UnsupportedSchema || itemCompleted.unsupported || directEvent.unsupported || responseFallback.unsupported
	}
	return result
}

func parseCodexLifecycleEvent(
	event map[string]any,
	payload map[string]any,
	sequence int,
	day time.Time,
	itemCompleted *codexEventTier,
	directEvent *codexEventTier,
	globalUnsupported *bool,
) {
	payloadType := strings.ToLower(normalizedText(payload["type"]))
	switch payloadType {
	case "user_message", "agent_message":
		message, ok := codexStringField(payload, "message")
		if !ok {
			directEvent.unsupported = true
			return
		}
		if payloadType == "user_message" {
			message = sanitizeCodexUserText(message)
		}
		appendCodexEvent(directEvent, message, event["timestamp"], sequence, day)
	case "item_completed":
		item, ok := payload["item"].(map[string]any)
		if !ok {
			*globalUnsupported = true
			return
		}
		itemType := normalizeCodexTurnItemType(item["type"])
		if itemType != "usermessage" && itemType != "agentmessage" {
			return
		}
		text, supported := codexTurnItemText(item, itemType == "usermessage")
		if !supported {
			itemCompleted.unsupported = true
			return
		}
		appendCodexEvent(itemCompleted, text, event["timestamp"], sequence, day)
	case "":
		*globalUnsupported = true
	default:
		// Tool, reasoning, plan, status, and other lifecycle events are not
		// user-authored or agent-facing conversation text.
	}
}

func parseCodexResponseFallback(
	event map[string]any,
	payload map[string]any,
	sequence int,
	day time.Time,
	tier *codexEventTier,
) {
	if strings.ToLower(normalizedText(payload["type"])) != "message" {
		return
	}
	role := strings.ToLower(normalizedText(payload["role"]))
	if role != "user" && role != "assistant" {
		return
	}

	text, supported := codexResponseContentText(payload["content"], role)
	if !supported {
		tier.unsupported = true
		return
	}
	appendCodexEvent(tier, text, event["timestamp"], sequence, day)
}

func appendCodexEvent(tier *codexEventTier, text string, timestampValue any, sequence int, day time.Time) {
	timestamp, ok := parseEventTimestamp(timestampValue)
	if !ok {
		tier.unsupported = true
		return
	}
	text = strings.TrimSpace(text)
	if text == "" || !isRequestedDay(timestamp, day) {
		return
	}
	tier.events = append(tier.events, sourceEvent{
		Text:      text,
		Timestamp: timestamp,
		Sequence:  sequence,
	})
}

func codexTurnItemText(item map[string]any, user bool) (string, bool) {
	blocks, ok := item["content"].([]any)
	if !ok {
		return "", false
	}
	texts := make([]string, 0, len(blocks))
	for _, value := range blocks {
		block, ok := value.(map[string]any)
		if !ok {
			return "", false
		}
		blockType := strings.ToLower(normalizedText(block["type"]))
		if blockType != "text" {
			// User messages may also contain images, skills, and mentions. Those
			// are intentionally excluded, as are any future non-text blocks.
			continue
		}
		text, ok := codexStringField(block, "text")
		if !ok {
			return "", false
		}
		if user {
			text = sanitizeCodexUserText(text)
		}
		if text = strings.TrimSpace(text); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n"), true
}

func codexResponseContentText(content any, role string) (string, bool) {
	blocks, ok := content.([]any)
	if !ok {
		return "", false
	}
	expectedType := "output_text"
	if role == "user" {
		expectedType = "input_text"
	}

	texts := make([]string, 0, len(blocks))
	for _, value := range blocks {
		block, ok := value.(map[string]any)
		if !ok {
			return "", false
		}
		if strings.ToLower(normalizedText(block["type"])) != expectedType {
			continue
		}
		text, ok := codexStringField(block, "text")
		if !ok {
			return "", false
		}
		if role == "user" {
			text = sanitizeCodexUserText(text)
		}
		if text = strings.TrimSpace(text); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n"), true
}

func codexStringField(value map[string]any, key string) (string, bool) {
	raw, exists := value[key]
	if !exists {
		return "", false
	}
	text, ok := raw.(string)
	return strings.TrimSpace(text), ok
}

func normalizeCodexTurnItemType(value any) string {
	typeName := strings.ToLower(normalizedText(value))
	typeName = strings.ReplaceAll(typeName, "_", "")
	typeName = strings.ReplaceAll(typeName, "-", "")
	return typeName
}

// sanitizeCodexUserText removes known host-injected context blocks without
// discarding a real user sentence that precedes an appended block. It is
// deliberately based on explicit boundary markers rather than generic prose.
func sanitizeCodexUserText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	earliest := len(text)
	for _, marker := range []string{
		"<environment_context>",
		"<world_state>",
		"<app-context>",
		"<permissions instructions>",
		"<user_instructions>",
		"<developer_instructions>",
		"<system_instructions>",
		"<skills_instructions>",
		"<apps_instructions>",
		"<plugins_instructions>",
		"<recommended_plugins>",
		"<collaboration_mode>",
		"<multi_agent_mode>",
		"# AGENTS.md instructions for ",
		"<INSTRUCTIONS>",
	} {
		if index := strings.Index(text, marker); index >= 0 && index < earliest {
			earliest = index
		}
	}
	if earliest < len(text) {
		text = strings.TrimSpace(text[:earliest])
	}
	return text
}
