package sources

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	cursorUserQueryOpen  = "<user_query>"
	cursorUserQueryClose = "</user_query>"
)

// matchCursorArtifact limits default discovery to Cursor's append-only agent
// transcripts. The old workspaceStorage database tree is intentionally not a
// transcript source: it mixes unrelated editor state with conversation data.
func matchCursorArtifact(root string, path string, direct bool) bool {
	if !strings.EqualFold(filepath.Ext(path), ".jsonl") || cursorHasPathComponent(path, "workspaceStorage") {
		return false
	}
	if direct {
		return candidateIdentity(root) == candidateIdentity(path)
	}

	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	components := strings.Split(relative, string(filepath.Separator))
	for index, component := range components {
		if component == "agent-transcripts" && index > 0 && index < len(components)-1 {
			return true
		}
	}
	return false
}

func cursorHasPathComponent(path string, target string) bool {
	cleaned := filepath.Clean(path)
	volume := filepath.VolumeName(cleaned)
	cleaned = strings.TrimPrefix(cleaned, volume)
	for _, component := range strings.Split(cleaned, string(filepath.Separator)) {
		if strings.EqualFold(component, target) {
			return true
		}
	}
	return false
}

// parseCursorArtifact understands only Cursor's transcript message envelope:
//
//	{"role":"user","message":{"content":[{"type":"text",...}]}}
//
// Newer records may place role under message and add type=message plus an RFC
// 3339 timestamp. Only one complete <user_query> wrapper is accepted. Cursor's
// public transcript shape does not reliably distinguish assistant final text
// from reasoning or injected skills, so every assistant record is excluded.
func parseCursorArtifact(path string, day time.Time) artifactResult {
	lines, parseError, truncated, unreadable := readArtifactJSONLines(path)
	result := artifactResult{
		ParseError: parseError,
		Truncated:  truncated,
		Unreadable: unreadable,
	}
	if unreadable {
		return result
	}

	var (
		fallbackTimestamp time.Time
		fallbackLoaded    bool
		fallbackUsable    bool
	)
	loadFallback := func() (time.Time, bool) {
		if fallbackLoaded {
			return fallbackTimestamp, fallbackUsable
		}
		fallbackLoaded = true
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			result.Unreadable = true
			return time.Time{}, false
		}
		fallbackTimestamp = info.ModTime()
		fallbackUsable = true
		return fallbackTimestamp, true
	}

	for _, line := range lines {
		record, ok := line.Value.(map[string]any)
		if !ok {
			result.UnsupportedSchema = true
			continue
		}
		recordAllowed, recordTypeValid := cursorRecordTypeAllowed(record)
		if !recordTypeValid {
			result.UnsupportedSchema = true
			continue
		}
		if !recordAllowed {
			continue
		}

		_, message, valid, excluded := cursorMessageEnvelope(record)
		if excluded {
			continue
		}
		if !valid {
			result.UnsupportedSchema = true
			continue
		}

		text, valid := cursorUserQueryText(message)
		if !valid {
			result.UnsupportedSchema = true
			continue
		}
		if text == "" {
			continue
		}

		timestamp, present, valid := cursorRecordTimestamp(record)
		timestampBasis := ""
		if !valid {
			result.UnsupportedSchema = true
			continue
		}
		if !present {
			var usable bool
			timestamp, usable = loadFallback()
			if !usable {
				continue
			}
			timestampBasis = "file_modified_at"
		}
		if !isRequestedDay(timestamp, day) {
			continue
		}

		result.Events = append(result.Events, sourceEvent{
			Text:           text,
			Timestamp:      timestamp,
			TimestampBasis: timestampBasis,
			Sequence:       line.Sequence,
		})
	}
	return result
}

func cursorRecordTypeAllowed(record map[string]any) (bool, bool) {
	raw, present := record["type"]
	if !present {
		return true, true
	}
	recordType, ok := raw.(string)
	if !ok || strings.TrimSpace(recordType) == "" {
		return false, false
	}
	// Current Cursor transcripts use type=message when a type is present.
	// Known non-conversation lifecycle records are ignored without inspecting
	// nested fields; a future type is a coverage gap, not verified no activity.
	switch recordType {
	case "message":
		return true, true
	case "system", "tool", "tool_call", "tool_result", "reasoning", "thinking", "skill", "skill_use":
		return false, true
	default:
		return false, false
	}
}

func cursorMessageEnvelope(record map[string]any) (string, map[string]any, bool, bool) {
	topRole, topRolePresent, topRoleValid := cursorOptionalString(record, "role")
	if !topRoleValid {
		return "", nil, false, false
	}
	if topRolePresent && topRole == "assistant" {
		return "", nil, false, true
	}
	if topRolePresent && cursorExcludedRole(topRole) {
		return "", nil, false, true
	}

	rawMessage, messagePresent := record["message"]
	message, messageValid := rawMessage.(map[string]any)
	if !messagePresent || !messageValid {
		return "", nil, false, false
	}
	nestedRole, nestedRolePresent, nestedRoleValid := cursorOptionalString(message, "role")
	if !nestedRoleValid {
		return "", nil, false, false
	}
	if nestedRolePresent && cursorExcludedRole(nestedRole) {
		if topRolePresent && topRole != nestedRole {
			return "", nil, false, false
		}
		return "", nil, false, true
	}
	if nestedRolePresent && nestedRole == "assistant" {
		if topRolePresent && topRole != nestedRole {
			return "", nil, false, false
		}
		return "", nil, false, true
	}

	if topRolePresent && nestedRolePresent && topRole != nestedRole {
		return "", nil, false, false
	}
	role := topRole
	if !topRolePresent {
		role = nestedRole
	}
	if role != "user" {
		return "", nil, false, false
	}
	return role, message, true, false
}

func cursorOptionalString(value map[string]any, key string) (string, bool, bool) {
	raw, present := value[key]
	if !present {
		return "", false, true
	}
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", true, false
	}
	return strings.TrimSpace(text), true, true
}

func cursorExcludedRole(role string) bool {
	switch role {
	case "system", "developer", "tool", "reasoning", "thinking", "skill":
		return true
	default:
		return false
	}
}

func cursorUserQueryText(message map[string]any) (string, bool) {
	rawContent, present := message["content"]
	content, ok := rawContent.([]any)
	if !present || !ok {
		return "", false
	}

	query := ""
	textBlocks := 0
	for _, rawBlock := range content {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			return "", false
		}
		blockType, present, valid := cursorOptionalString(block, "type")
		if !present || !valid {
			return "", false
		}
		if blockType != "text" {
			continue
		}
		textBlocks++
		if textBlocks > 1 {
			return "", false
		}
		text, present, valid := cursorOptionalString(block, "text")
		if !present || !valid {
			return "", false
		}
		text, valid = unwrapCursorUserQuery(text)
		if !valid {
			return "", false
		}
		query = text
	}
	if textBlocks != 1 || query == "" {
		return "", false
	}
	return query, true
}

func unwrapCursorUserQuery(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, cursorUserQueryOpen) || !strings.HasSuffix(trimmed, cursorUserQueryClose) {
		return "", false
	}
	if strings.Count(trimmed, cursorUserQueryOpen) != 1 || strings.Count(trimmed, cursorUserQueryClose) != 1 {
		return "", false
	}
	query := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, cursorUserQueryOpen), cursorUserQueryClose))
	return query, query != ""
}

func cursorRecordTimestamp(record map[string]any) (time.Time, bool, bool) {
	raw, present := record["timestamp"]
	if !present {
		return time.Time{}, false, true
	}
	timestamp, ok := parseEventTimestamp(raw)
	return timestamp, true, ok
}
