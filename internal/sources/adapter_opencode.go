package sources

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// matchOpenCodeArtifact deliberately does not scan OpenCode's generic data,
// log, auth, or configuration files. Current OpenCode stores conversations in
// opencode.db; the database is detected so callers can report that storage as
// unsupported without opening it. The JSON allowlist is limited to the legacy
// text-part layouts that OpenCode itself migrated from.
func matchOpenCodeArtifact(root string, path string, direct bool) bool {
	if direct && candidateIdentity(root) != candidateIdentity(path) {
		return false
	}
	if isOpenCodeDatabase(path) {
		if direct {
			return true
		}
		relative, ok := openCodeRelativePath(root, path)
		return ok && filepath.Dir(relative) == "."
	}

	extension := strings.ToLower(filepath.Ext(path))
	if extension != ".json" && extension != ".jsonl" {
		return false
	}
	if hasOpenCodePrivatePath(path) {
		return false
	}
	if direct {
		return !isOpenCodeSensitiveFile(filepath.Base(path))
	}

	relative, ok := openCodeRelativePath(root, path)
	if !ok {
		return false
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) == 4 && strings.EqualFold(parts[0], "storage") && strings.EqualFold(parts[1], "part") {
		return openCodeID(parts[2], "msg_") && openCodeJSONID(parts[3], "prt_")
	}
	// Pre-migration layout:
	// project/<project>/storage/session/part/<session>/<message>/<part>.json
	if len(parts) == 8 && strings.EqualFold(parts[0], "project") &&
		strings.EqualFold(parts[2], "storage") && strings.EqualFold(parts[3], "session") &&
		strings.EqualFold(parts[4], "part") {
		return parts[1] != "" && openCodeID(parts[5], "ses_") &&
			openCodeID(parts[6], "msg_") && openCodeJSONID(parts[7], "prt_")
	}
	return false
}

func openCodeRelativePath(root string, path string) (string, bool) {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Clean(relative), true
}

func isOpenCodeDatabase(path string) bool {
	return strings.EqualFold(filepath.Base(path), "opencode.db")
}

func isOpenCodeSensitiveFile(name string) bool {
	switch strings.ToLower(name) {
	case "auth.json", "config.json", "opencode.json", "opencode.jsonc":
		return true
	default:
		return false
	}
}

func hasOpenCodePrivatePath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(path)), "/") {
		switch strings.ToLower(part) {
		case "log", "logs", "auth", "config":
			return true
		}
	}
	return false
}

func openCodeID(value string, prefix string) bool {
	return strings.HasPrefix(value, prefix) && len(value) > len(prefix)
}

func openCodeJSONID(value string, prefix string) bool {
	return strings.EqualFold(filepath.Ext(value), ".json") && openCodeID(strings.TrimSuffix(value, filepath.Ext(value)), prefix)
}

// parseOpenCodeArtifact extracts only visible text parts from recognized
// OpenCode legacy storage, message exports, and `opencode run --format json`
// event streams. It never falls back to generic text-bearing JSON keys.
func parseOpenCodeArtifact(path string, day time.Time) artifactResult {
	if isOpenCodeDatabase(path) {
		return artifactResult{UnsupportedStorage: true}
	}

	result := artifactResult{}
	sequence := 0
	fallback, fallbackOK := openCodeArtifactModTime(path)
	appendValue := func(value any, allowMtimeFallback bool) {
		events, recognized, unsupported := openCodeEvents(value, day, fallback, fallbackOK && allowMtimeFallback, &sequence)
		result.Events = append(result.Events, events...)
		if !recognized || unsupported {
			result.UnsupportedSchema = true
		}
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".jsonl":
		lines, parseError, truncated, unreadable := readArtifactJSONLines(path)
		result.ParseError = parseError
		result.Truncated = truncated
		result.Unreadable = unreadable
		for _, line := range lines {
			appendValue(line.Value, false)
		}
	case ".json":
		value, parseError, truncated, unreadable := readArtifactJSON(path)
		result.ParseError = parseError
		result.Truncated = truncated
		result.Unreadable = unreadable
		if truncated && value == nil {
			result.UnsupportedSchema = true
		}
		if !parseError && !truncated && !unreadable && value != nil {
			appendValue(value, true)
		}
	default:
		result.UnsupportedSchema = true
	}
	return result
}

func openCodeArtifactModTime(path string) (time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

func openCodeEvents(value any, day time.Time, fallback time.Time, allowMtimeFallback bool, sequence *int) ([]sourceEvent, bool, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if messages, exists := typed["messages"]; exists {
			items, ok := messages.([]any)
			if !ok {
				return nil, false, true
			}
			if len(items) == 0 {
				return nil, openCodeSessionMetadata(typed["info"]), !openCodeSessionMetadata(typed["info"])
			}
			events := make([]sourceEvent, 0, len(items))
			unsupported := false
			for _, item := range items {
				messageEvents, recognized, itemUnsupported := openCodeMessageEvents(item, day, sequence)
				if !recognized || itemUnsupported {
					unsupported = true
				}
				events = append(events, messageEvents...)
			}
			return events, true, unsupported
		}
		if _, hasInfo := typed["info"]; hasInfo {
			return openCodeMessageEvents(typed, day, sequence)
		}
		if openCodeID(normalizedText(typed["id"]), "ses_") {
			// Session metadata is recognizable but never evidence. In
			// particular, do not expose its title, directory, or share URL.
			if openCodeSessionMetadata(typed) {
				return nil, true, false
			}
		}
		if _, hasPart := typed["part"]; hasPart || (normalizedText(typed["type"]) != "" && !openCodePartIdentity(typed)) {
			return openCodeStreamEvent(typed, day, sequence)
		}
		return openCodeStoredPart(typed, day, fallback, allowMtimeFallback, sequence)
	case []any:
		if len(typed) == 0 {
			return nil, false, true
		}
		events := make([]sourceEvent, 0, len(typed))
		unsupported := false
		for _, item := range typed {
			itemEvents, recognized, itemUnsupported := openCodeEvents(item, day, fallback, false, sequence)
			if !recognized || itemUnsupported {
				unsupported = true
			}
			events = append(events, itemEvents...)
		}
		return events, true, unsupported
	default:
		return nil, false, true
	}
}

func openCodeStreamEvent(event map[string]any, day time.Time, sequence *int) ([]sourceEvent, bool, bool) {
	eventType := strings.ToLower(normalizedText(event["type"]))
	sessionID := normalizedText(event["sessionID"])
	if !openCodeID(sessionID, "ses_") || eventType == "" {
		return nil, false, true
	}
	if eventType == "error" {
		_, timestampOK := parseEventTimestamp(event["timestamp"])
		_, hasError := event["error"]
		return nil, true, !timestampOK || !hasError
	}
	part, ok := event["part"].(map[string]any)
	if !ok {
		return nil, false, true
	}
	if normalizedText(part["sessionID"]) != sessionID || !openCodePartIdentity(part) {
		return nil, false, true
	}

	partType := strings.ToLower(normalizedText(part["type"]))
	if eventType != "text" {
		if openCodeNonTextEvent(eventType, partType) {
			return nil, true, false
		}
		return nil, true, true
	}
	if partType != "text" {
		return nil, true, true
	}
	timestamp, ok := parseEventTimestamp(event["timestamp"], openCodePartTime(part, "end"), openCodePartTime(part, "start"))
	if !ok {
		return nil, true, true
	}
	return openCodeTextPartEvent(part, timestamp, "", day, sequence)
}

func openCodeNonTextEvent(eventType string, partType string) bool {
	switch eventType {
	case "tool_use":
		return partType == "tool"
	case "step_start":
		return partType == "step-start"
	case "step_finish":
		return partType == "step-finish"
	case "reasoning":
		return partType == "reasoning"
	default:
		return false
	}
}

func openCodeStoredPart(part map[string]any, day time.Time, fallback time.Time, allowMtimeFallback bool, sequence *int) ([]sourceEvent, bool, bool) {
	if !openCodePartIdentity(part) {
		return nil, false, true
	}
	partType := strings.ToLower(normalizedText(part["type"]))
	if partType != "text" {
		return nil, true, !openCodeKnownPartType(partType)
	}
	timestamp, ok := parseEventTimestamp(openCodePartTime(part, "end"), openCodePartTime(part, "start"), part["timestamp"])
	timestampBasis := ""
	if !ok && allowMtimeFallback && !fallback.IsZero() {
		timestamp, ok = fallback, true
		timestampBasis = "file_modified_at"
	}
	if !ok {
		return nil, true, true
	}
	return openCodeTextPartEvent(part, timestamp, timestampBasis, day, sequence)
}

func openCodeMessageEvents(value any, day time.Time, sequence *int) ([]sourceEvent, bool, bool) {
	message, ok := value.(map[string]any)
	if !ok {
		return nil, false, true
	}
	info, ok := message["info"].(map[string]any)
	if !ok {
		return nil, false, true
	}
	messageID := normalizedText(info["id"])
	sessionID := normalizedText(info["sessionID"])
	role := strings.ToLower(normalizedText(info["role"]))
	if !openCodeID(messageID, "msg_") || !openCodeID(sessionID, "ses_") || (role != "user" && role != "assistant") {
		return nil, false, true
	}
	timestamp, timestampOK := parseEventTimestamp(openCodePartTime(info, "created"), info["timestamp"], info["created_at"])
	if !timestampOK {
		return nil, true, true
	}
	parts, ok := message["parts"].([]any)
	if !ok {
		return nil, true, true
	}

	events := make([]sourceEvent, 0, len(parts))
	unsupported := false
	for _, item := range parts {
		part, ok := item.(map[string]any)
		if !ok || !openCodePartIdentity(part) || normalizedText(part["messageID"]) != messageID || normalizedText(part["sessionID"]) != sessionID {
			unsupported = true
			continue
		}
		partType := strings.ToLower(normalizedText(part["type"]))
		if partType != "text" {
			if !openCodeKnownPartType(partType) {
				unsupported = true
			}
			continue
		}
		partTimestamp, ok := parseEventTimestamp(openCodePartTime(part, "end"), openCodePartTime(part, "start"))
		if !ok {
			partTimestamp = timestamp
		}
		partEvents, _, partUnsupported := openCodeTextPartEvent(part, partTimestamp, "", day, sequence)
		unsupported = unsupported || partUnsupported
		events = append(events, partEvents...)
	}
	return events, true, unsupported
}

func openCodePartIdentity(part map[string]any) bool {
	return openCodeID(normalizedText(part["id"]), "prt_") &&
		openCodeID(normalizedText(part["messageID"]), "msg_") &&
		openCodeID(normalizedText(part["sessionID"]), "ses_")
}

func openCodeKnownPartType(partType string) bool {
	switch partType {
	case "text", "tool", "file", "reasoning", "step-start", "step-finish", "snapshot", "patch", "agent", "retry", "compaction", "subtask":
		return true
	default:
		return false
	}
}

func openCodeSessionMetadata(value any) bool {
	session, ok := value.(map[string]any)
	if !ok || !openCodeID(normalizedText(session["id"]), "ses_") {
		return false
	}
	if normalizedText(session["title"]) == "" || normalizedText(session["directory"]) == "" {
		return false
	}
	_, ok = parseEventTimestamp(openCodePartTime(session, "created"), session["created_at"])
	return ok
}

func openCodePartTime(value map[string]any, key string) any {
	times, ok := value["time"].(map[string]any)
	if !ok {
		return nil
	}
	return times[key]
}

func openCodeTextPartEvent(part map[string]any, timestamp time.Time, timestampBasis string, day time.Time, sequence *int) ([]sourceEvent, bool, bool) {
	if flag, ok := part["synthetic"].(bool); ok && flag {
		return nil, true, false
	}
	if flag, ok := part["ignored"].(bool); ok && flag {
		return nil, true, false
	}
	textValue, exists := part["text"]
	if !exists {
		return nil, true, true
	}
	text, ok := textValue.(string)
	if !ok {
		return nil, true, true
	}
	text = strings.TrimSpace(text)
	if text == "" || !isRequestedDay(timestamp, day) {
		return nil, true, false
	}
	event := sourceEvent{Text: text, Timestamp: timestamp, TimestampBasis: timestampBasis, Sequence: *sequence}
	*sequence++
	return []sourceEvent{event}, true, false
}
