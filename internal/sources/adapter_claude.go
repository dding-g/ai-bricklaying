package sources

import (
	"path/filepath"
	"strings"
	"time"
)

// matchClaudeArtifact accepts only the top-level JSONL transcripts stored as
// <projects-root>/<project>/<session>.jsonl. Claude's nested subagent, tool
// result, and memory artifacts are deliberately outside the conversation
// allowlist even when a custom root points at them.
func matchClaudeArtifact(root string, path string, direct bool) bool {
	if !strings.EqualFold(filepath.Ext(path), ".jsonl") {
		return false
	}
	if direct {
		return candidateIdentity(root) == candidateIdentity(path) && !hasClaudePrivatePathSegment(path)
	}

	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	if len(parts) != 2 || hasClaudePrivatePathSegment(relative) {
		return false
	}
	return parts[0] != "" && parts[1] != ""
}

func hasClaudePrivatePathSegment(path string) bool {
	for _, part := range strings.FieldsFunc(filepath.ToSlash(filepath.Clean(path)), func(value rune) bool {
		return value == '/'
	}) {
		switch strings.ToLower(part) {
		case "subagents", "tool-results", "tool_results", "memory":
			return true
		}
	}
	return false
}

// parseClaudeArtifact extracts only user/assistant conversation text. It never
// falls back to raw JSON because Claude transcripts also contain tool inputs,
// tool results, system prompts, compact summaries, and sidechain activity.
func parseClaudeArtifact(path string, day time.Time) artifactResult {
	lines, parseError, truncated, unreadable := readArtifactJSONLines(path)
	result := artifactResult{
		ParseError: parseError,
		Truncated:  truncated,
		Unreadable: unreadable,
	}
	for _, line := range lines {
		event, ok := line.Value.(map[string]any)
		if !ok {
			result.UnsupportedSchema = true
			continue
		}

		eventType := strings.ToLower(normalizedText(event["type"]))
		if eventType != "user" && eventType != "assistant" {
			if !knownClaudeNonConversationType(eventType) {
				result.UnsupportedSchema = true
			}
			continue
		}
		if shouldExcludeClaudeEvent(event) {
			continue
		}

		message, direct, ok := claudeMessage(event)
		if !ok {
			result.UnsupportedSchema = true
			continue
		}
		if shouldExcludeClaudeEvent(message) {
			continue
		}

		role := strings.ToLower(normalizedText(message["role"]))
		if direct {
			if role != "" && role != eventType {
				result.UnsupportedSchema = true
				continue
			}
		} else if role != eventType {
			result.UnsupportedSchema = true
			continue
		}

		timestamp, ok := parseEventTimestamp(event["timestamp"], message["timestamp"], event["created_at"], message["created_at"])
		if !ok {
			result.UnsupportedSchema = true
			continue
		}
		if !isRequestedDay(timestamp, day) {
			continue
		}

		text, supported := claudeContentText(message["content"])
		if !supported {
			result.UnsupportedSchema = true
			continue
		}
		if text == "" {
			continue
		}
		result.Events = append(result.Events, sourceEvent{
			Text:      text,
			Timestamp: timestamp,
			Sequence:  line.Sequence,
		})
	}
	return result
}

func knownClaudeNonConversationType(eventType string) bool {
	switch eventType {
	case "system", "summary", "progress", "queue-operation", "file-history-snapshot", "hook_progress":
		return true
	default:
		return false
	}
}

func claudeMessage(event map[string]any) (message map[string]any, direct bool, ok bool) {
	if value, exists := event["message"]; exists {
		message, ok = value.(map[string]any)
		return message, false, ok
	}
	if _, exists := event["content"]; !exists {
		return nil, false, false
	}
	return event, true, true
}

func shouldExcludeClaudeEvent(value map[string]any) bool {
	for _, key := range []string{"isMeta", "isSidechain", "isCompactSummary", "isApiErrorMessage"} {
		if flag, ok := value[key].(bool); ok && flag {
			return true
		}
	}
	for _, key := range []string{"error", "errors"} {
		if hasClaudeErrorValue(value[key]) {
			return true
		}
	}
	return false
}

func hasClaudeErrorValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case bool:
		return typed
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func claudeContentText(content any) (string, bool) {
	if text, ok := content.(string); ok {
		return strings.TrimSpace(text), true
	}
	blocks, ok := content.([]any)
	if !ok {
		return "", false
	}

	texts := make([]string, 0, len(blocks))
	for _, value := range blocks {
		block, ok := value.(map[string]any)
		if !ok {
			return "", false
		}
		if strings.ToLower(normalizedText(block["type"])) != "text" {
			continue
		}
		textValue, exists := block["text"]
		if !exists {
			return "", false
		}
		text, ok := textValue.(string)
		if !ok {
			return "", false
		}
		if text = strings.TrimSpace(text); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n"), true
}
