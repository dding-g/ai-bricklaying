package safeio

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

const Redacted = "[REDACTED]"

var (
	controlPattern      = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f\x9b]`)
	slackWebhookPattern = regexp.MustCompile(`https://hooks\.slack\.com/services/[^\s)\]}'"` + "`" + `<>]+`)
	privateKeyPattern   = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	bearerPattern       = regexp.MustCompile(`(?i)\bBearer\s+[^\s,;]+`)
	keyValuePattern     = regexp.MustCompile(`(?i)(["']?(?:token|api[_-]?key|access[_-]?key|secret|password|private[_-]?key|webhook(?:[_-]?url)?)["']?\s*[:=]\s*)["']?[^\s,;}"']+`)
	longTokenPattern    = regexp.MustCompile(`\b(?:sk|pk)(?:_[A-Za-z0-9]{4,})+\b`)
	sensitiveKeyPattern = regexp.MustCompile(`(?i)(^|[_-])(password|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|webhook(?:[_-]?url)?)([_-]|$)`)
)

func SanitizeControl(value string) string {
	return controlPattern.ReplaceAllString(value, "")
}

func RedactString(value string) string {
	redacted := SanitizeControl(value)
	redacted = slackWebhookPattern.ReplaceAllString(redacted, "[REDACTED SLACK WEBHOOK]")
	redacted = privateKeyPattern.ReplaceAllString(redacted, "[REDACTED PRIVATE KEY]")
	redacted = bearerPattern.ReplaceAllString(redacted, "Bearer "+Redacted)
	redacted = keyValuePattern.ReplaceAllString(redacted, "${1}"+Redacted)
	redacted = longTokenPattern.ReplaceAllString(redacted, "[REDACTED TOKEN]")
	return redacted
}

func RedactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, item := range typed {
			if IsSensitiveKey(key) {
				redacted[key] = Redacted
				continue
			}
			redacted[key] = RedactValue(item)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for index, item := range typed {
			redacted[index] = RedactValue(item)
		}
		return redacted
	case string:
		return RedactString(typed)
	default:
		return typed
	}
}

func RedactJSON(input []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(RedactValue(value))
}

func IsSensitiveKey(key string) bool {
	normalized := strings.TrimSpace(key)
	return sensitiveKeyPattern.MatchString(normalized)
}
