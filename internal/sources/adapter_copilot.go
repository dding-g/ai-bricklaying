package sources

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"time"
)

const (
	copilotMutationInitial = 0
	copilotMutationSet     = 1
	copilotMutationPush    = 2
	copilotMutationDelete  = 3

	copilotSessionVersion = 3
	copilotMaxPathDepth   = 64
	copilotExtensionID    = "GitHub.copilot-chat"
)

// matchCopilotArtifact accepts only VS Code's workspace-scoped mutation logs:
// <workspaceStorage>/<workspace-id>/chatSessions/<session-id>.jsonl. A directly
// configured file is an explicit opt-in, but parseCopilotArtifact still
// requires the same mutation and session schemas before exposing any text.
func matchCopilotArtifact(root string, path string, direct bool) bool {
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
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	if len(parts) != 3 || parts[0] == "" || parts[1] != "chatSessions" {
		return false
	}
	return strings.TrimSuffix(parts[2], filepath.Ext(parts[2])) != ""
}

// parseCopilotArtifact replays VS Code's append-only ObjectMutationLog and
// extracts only user request text and serialized MarkdownString response text.
// The rest of a chat session can contain system-initiated requests, tool calls,
// thinking blocks, command payloads, context, edits, and result metadata, none
// of which are conversation evidence.
func parseCopilotArtifact(path string, day time.Time) artifactResult {
	lines, parseError, truncated, unreadable := readArtifactJSONLines(path)
	result := artifactResult{
		ParseError: parseError,
		Truncated:  truncated,
		Unreadable: unreadable,
	}
	if unreadable || parseError {
		// A skipped mutation can change every later path. Replaying the remaining
		// lines would produce a state that never existed, so fail closed.
		return result
	}
	if len(lines) == 0 {
		result.UnsupportedSchema = true
		return result
	}

	var state any
	initialized := false
	for _, line := range lines {
		entry, ok := line.Value.(map[string]any)
		if !ok {
			result.UnsupportedSchema = true
			return result
		}
		kind, ok := copilotInteger(entry["kind"])
		if !ok {
			result.UnsupportedSchema = true
			return result
		}

		switch kind {
		case copilotMutationInitial:
			value, exists := entry["v"]
			if initialized || !exists {
				result.UnsupportedSchema = true
				return result
			}
			if _, ok := value.(map[string]any); !ok {
				result.UnsupportedSchema = true
				return result
			}
			state = value
			initialized = true

		case copilotMutationSet, copilotMutationPush, copilotMutationDelete:
			if !initialized {
				result.UnsupportedSchema = true
				return result
			}
			path, ok := copilotMutationPath(entry["k"])
			if !ok || len(path) == 0 {
				result.UnsupportedSchema = true
				return result
			}
			var applied bool
			switch kind {
			case copilotMutationSet:
				value, exists := entry["v"]
				applied = exists && copilotAssign(state, path, value, false)
			case copilotMutationDelete:
				applied = copilotAssign(state, path, nil, true)
			case copilotMutationPush:
				applied = copilotPush(state, path, entry)
			}
			if !applied {
				result.UnsupportedSchema = true
				return result
			}

		default:
			result.UnsupportedSchema = true
			return result
		}
	}
	if !initialized {
		result.UnsupportedSchema = true
		return result
	}

	session, ok := state.(map[string]any)
	if !ok {
		result.UnsupportedSchema = true
		return result
	}
	events, unsupported := copilotSessionEvents(session, day)
	result.Events = events
	result.UnsupportedSchema = unsupported
	return result
}

type copilotPathPart struct {
	key   string
	index int
	isKey bool
}

func copilotMutationPath(value any) ([]copilotPathPart, bool) {
	raw, ok := value.([]any)
	if !ok || len(raw) == 0 || len(raw) > copilotMaxPathDepth {
		return nil, false
	}
	path := make([]copilotPathPart, 0, len(raw))
	for _, segment := range raw {
		if key, ok := segment.(string); ok && key != "" {
			path = append(path, copilotPathPart{key: key, isKey: true})
			continue
		}
		index, ok := copilotInteger(segment)
		if !ok || index < 0 {
			return nil, false
		}
		path = append(path, copilotPathPart{index: index})
	}
	return path, true
}

func copilotInteger(value any) (int, bool) {
	var parsed int64
	switch typed := value.(type) {
	case json.Number:
		value, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		parsed = value
	case int:
		return typed, true
	case int64:
		parsed = typed
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed {
			return 0, false
		}
		parsed = int64(typed)
	default:
		return 0, false
	}
	converted := int(parsed)
	if int64(converted) != parsed {
		return 0, false
	}
	return converted, true
}

func copilotResolve(root any, path []copilotPathPart) (any, bool) {
	current := root
	for _, part := range path {
		if part.isKey {
			object, ok := current.(map[string]any)
			if !ok {
				return nil, false
			}
			value, exists := object[part.key]
			if !exists {
				return nil, false
			}
			current = value
			continue
		}
		array, ok := current.([]any)
		if !ok || part.index < 0 || part.index >= len(array) {
			return nil, false
		}
		current = array[part.index]
	}
	return current, true
}

func copilotAssign(root any, path []copilotPathPart, value any, remove bool) bool {
	parent, ok := copilotResolve(root, path[:len(path)-1])
	if !ok {
		return false
	}
	last := path[len(path)-1]
	if last.isKey {
		object, ok := parent.(map[string]any)
		if !ok {
			return false
		}
		if remove {
			delete(object, last.key)
		} else {
			object[last.key] = value
		}
		return true
	}

	array, ok := parent.([]any)
	if !ok || last.index < 0 || last.index >= len(array) {
		return false
	}
	// JavaScript delete/set-undefined leaves an array slot rather than
	// shifting later entries. nil is the safe JSON-equivalent hole here.
	array[last.index] = value
	return true
}

func copilotPush(root any, path []copilotPathPart, entry map[string]any) bool {
	current, exists := copilotResolve(root, path)
	if !exists || current == nil {
		current = []any{}
	}
	array, ok := current.([]any)
	if !ok {
		return false
	}
	array = append([]any(nil), array...)

	if rawIndex, hasIndex := entry["i"]; hasIndex {
		index, ok := copilotInteger(rawIndex)
		if !ok || index < 0 || index > len(array) {
			return false
		}
		array = array[:index]
	}
	if rawValues, hasValues := entry["v"]; hasValues {
		values, ok := rawValues.([]any)
		if !ok {
			return false
		}
		array = append(array, values...)
	} else if _, hasIndex := entry["i"]; !hasIndex {
		return false
	}
	return copilotAssign(root, path, array, false)
}

func copilotSessionEvents(session map[string]any, day time.Time) ([]sourceEvent, bool) {
	version, ok := copilotInteger(session["version"])
	if !ok || version != copilotSessionVersion {
		return nil, true
	}
	requests, ok := session["requests"].([]any)
	if !ok {
		return nil, true
	}

	creationDate, creationOK := parseEventTimestamp(session["creationDate"])
	unsupported := !creationOK
	events := make([]sourceEvent, 0, len(requests)*2)
	for index, value := range requests {
		request, ok := value.(map[string]any)
		if !ok {
			unsupported = true
			continue
		}
		if !copilotRequestAgent(request) {
			// chatSessions is a shared VS Code store. Its location alone does not
			// prove that an entry came from GitHub Copilot.
			unsupported = true
			continue
		}
		excluded, supported := copilotRequestExcluded(request)
		if !supported {
			unsupported = true
			continue
		}
		if excluded {
			continue
		}

		message, ok := request["message"].(map[string]any)
		if !ok {
			unsupported = true
			continue
		}
		textValue, exists := message["text"]
		requestText, textOK := textValue.(string)
		if !exists || !textOK {
			unsupported = true
			continue
		}
		requestText = strings.TrimSpace(requestText)

		requestTimestamp, timestampOK, timestampInvalid := copilotTimestamp(request, "timestamp", creationDate, creationOK)
		if timestampInvalid {
			unsupported = true
		}
		if requestText != "" && timestampOK && isRequestedDay(requestTimestamp, day) {
			events = append(events, sourceEvent{
				Text:      requestText,
				Timestamp: requestTimestamp,
				Sequence:  index * 2,
			})
		} else if requestText != "" && !timestampOK {
			unsupported = true
		}

		responseValue, hasResponse := request["response"]
		if !hasResponse || responseValue == nil {
			continue
		}
		response, ok := responseValue.([]any)
		if !ok {
			unsupported = true
			continue
		}
		responseText, responseSupported := copilotResponseText(response)
		if !responseSupported {
			unsupported = true
		}
		if responseText == "" {
			continue
		}

		responseTimestamp, responseTimestampOK, responseTimestampInvalid := copilotTimestamp(request, "responseTimestamp", requestTimestamp, timestampOK)
		if responseTimestampInvalid {
			unsupported = true
		}
		if responseTimestampOK && isRequestedDay(responseTimestamp, day) {
			events = append(events, sourceEvent{
				Text:      responseText,
				Timestamp: responseTimestamp,
				Sequence:  index*2 + 1,
			})
		} else if !responseTimestampOK {
			unsupported = true
		}
	}
	return events, unsupported
}

func copilotRequestAgent(request map[string]any) bool {
	agent, ok := request["agent"].(map[string]any)
	if !ok {
		return false
	}
	extensionID, exists := agent["extensionId"]
	if !exists {
		return false
	}
	// ExtensionIdentifier is currently serialized as {value, _lower}. Accept
	// the older direct-string representation only at this exact field.
	if value, ok := extensionID.(string); ok {
		return value == copilotExtensionID
	}
	identifier, ok := extensionID.(map[string]any)
	if !ok || normalizedText(identifier["value"]) != copilotExtensionID {
		return false
	}
	if lower, exists := identifier["_lower"]; exists && normalizedText(lower) != strings.ToLower(copilotExtensionID) {
		return false
	}
	return true
}

func copilotRequestExcluded(request map[string]any) (excluded bool, supported bool) {
	for _, key := range []string{"isSystemInitiated", "isHidden", "shouldBeRemovedOnSend"} {
		value, exists := request[key]
		if !exists || value == nil {
			continue
		}
		flag, ok := value.(bool)
		if !ok {
			return false, false
		}
		if flag {
			return true, true
		}
	}
	return false, true
}

func copilotTimestamp(value map[string]any, key string, fallback time.Time, fallbackOK bool) (time.Time, bool, bool) {
	raw, exists := value[key]
	if !exists {
		return fallback, fallbackOK, false
	}
	timestamp, ok := parseEventTimestamp(raw)
	if !ok {
		return time.Time{}, false, true
	}
	return timestamp, true, false
}

func copilotResponseText(parts []any) (string, bool) {
	texts := make([]string, 0, len(parts))
	supported := true
	for _, value := range parts {
		part, ok := value.(map[string]any)
		if !ok {
			supported = false
			continue
		}
		kind := normalizedText(part["kind"])
		if kind != "" {
			if kind == "markdownContent" {
				text, ok := copilotMarkdownContent(part["content"])
				if !ok {
					supported = false
					continue
				}
				if text != "" {
					texts = append(texts, text)
				}
				continue
			}
			if !copilotKnownNonTextResponseKind(kind) {
				supported = false
			}
			continue
		}

		text, ok := copilotMarkdownString(part)
		if !ok {
			supported = false
			continue
		}
		if text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n"), supported
}

func copilotMarkdownContent(value any) (string, bool) {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text), true
	}
	object, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	return copilotMarkdownString(object)
}

func copilotMarkdownString(value map[string]any) (string, bool) {
	for key := range value {
		switch key {
		case "value", "isTrusted", "supportThemeIcons", "supportHtml", "supportAlertSyntax", "baseUri", "uris":
		default:
			return "", false
		}
	}
	raw, exists := value["value"]
	text, ok := raw.(string)
	if !exists || !ok {
		return "", false
	}
	return strings.TrimSpace(text), true
}

func copilotKnownNonTextResponseKind(kind string) bool {
	switch kind {
	case "toolInvocationSerialized", "elicitationSerialized", "progressTaskSerialized",
		"textEditGroup", "multiDiffData", "mcpServersStarting", "thinking",
		"clearToPreviousToolInvocation", "codeblockUri", "command", "confirmation",
		"extensions", "hook", "inlineReference", "markdownVuln", "notebookEditGroup",
		"progressMessage", "systemNotification", "pullRequest", "questionCarousel",
		"planReview", "undoStop", "warning", "info", "treeData", "workspaceEdit",
		"externalEdit", "disabledClaudeHooks", "autoModeResolution":
		return true
	default:
		return false
	}
}
