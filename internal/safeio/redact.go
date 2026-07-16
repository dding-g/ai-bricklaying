package safeio

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

const Redacted = "[REDACTED]"

var (
	controlPattern        = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f\x9b]`)
	slackWebhookPattern   = regexp.MustCompile(`(?i)https:(?:(?:\\/)|/){2}hooks\.slack\.com(?:(?:\\/)|/)services(?:(?:\\/)|/)[^\s)\]}'"` + "`" + `<>]+`)
	privateKeyPattern     = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	credentialURLPattern  = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*:(?:(?:\\/)|/){2})[^/\\\s:@]+:[^/\\\s@]+@`)
	bearerPattern         = regexp.MustCompile(`(?i)\bBearer(?:[ \t]|%20)+[^\s,;]+`)
	basicAuthPattern      = regexp.MustCompile(`(?i)\bBasic(?:[ \t]|%20)+[^\s,;]+`)
	githubTokenPattern    = regexp.MustCompile(`(?i)\b(?:gh[pousr]_[A-Za-z0-9]{12,255}|github_pat_[A-Za-z0-9_]{12,255})\b`)
	openAITokenPattern    = regexp.MustCompile(`(?i)\bsk-(?:(?:proj|svcacct)-)?[A-Za-z0-9_-]{12,255}`)
	awsAccessKeyPattern   = regexp.MustCompile(`\b(?:AKIA|ASIA|AIDA|AROA|AIPA|ANPA|ANVA|ASCA)[A-Z0-9]{16}\b`)
	jwtPattern            = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}`)
	keyValuePattern       = regexp.MustCompile(`(?im)(["']?(?:token|access[_-]?token|refresh[_-]?token|api[_-]?key|access[_-]?key(?:[_-]?id)?|secret|client[_-]?secret|password|passwd|private[_-]?key|webhook(?:[_-]?url)?|authorization|proxy[_-]?authorization|cookie|set[_-]?cookie|session(?:[_-]?(?:id|token|key))?|credentials?|csrf(?:[_-]?token)?|xsrf(?:[_-]?token)?)["']?[ \t]*[:=][ \t]*)[^\r\n]+`)
	longTokenPattern      = regexp.MustCompile(`\b(?:sk|pk)(?:_[A-Za-z0-9]{4,})+\b`)
	encodedSlackHint      = regexp.MustCompile(`(?i)https%3a(?:%2f){2}hooks(?:\.|%2e)slack(?:\.|%2e)com%2fservices%2f`)
	encodedCredentialHint = regexp.MustCompile(`(?i)(?:authorization|proxy(?:_|-|%5f|%2d)?authorization|cookie|set(?:_|-|%5f|%2d)?cookie|session(?:_|-|%5f|%2d)?(?:id|token|key)?|token|api(?:_|-|%5f|%2d)?key|password|passwd|secret)(?:\s|%20)*(?:%3a|%3d)|(?:github(?:_|%5f)pat(?:_|%5f)|gh[pousr](?:_|%5f)|sk(?:-|%2d)(?:(?:proj|svcacct)(?:-|%2d))?)[A-Za-z0-9%_-]{8,}|eyJ[A-Za-z0-9_-]{5,}(?:\.|%2e)[A-Za-z0-9_-]{5,}(?:\.|%2e)[A-Za-z0-9_%=-]{5,}|[a-z][a-z0-9+.-]*(?::|%3a)(?:(?:/|%2f){2})[^\s/]*(?:%3a)[^\s/]*(?:%40)`)
	escapedCredentialHint = regexp.MustCompile(`(?i)\\["'](?:token|access[_-]?token|refresh[_-]?token|api[_-]?key|secret|password|authorization|cookie|session(?:[_-]?(?:id|token|key))?)\\["']\s*[:=]`)
	sensitiveKeyPattern   = regexp.MustCompile(`(?i)^(?:.*[_-])?(?:password|passwd|secret|token|api[_-]?key|access[_-]?key(?:[_-]?id)?|private[_-]?key|webhook(?:[_-]?url)?|authorization|proxy[_-]?authorization|cookie|set[_-]?cookie|session[_-]?(?:id|token|key)|credentials?|csrf[_-]?token|xsrf[_-]?token)$`)
)

func SanitizeControl(value string) string {
	return controlPattern.ReplaceAllString(value, "")
}

func RedactString(value string) string {
	redacted := SanitizeControl(value)
	redacted = slackWebhookPattern.ReplaceAllString(redacted, "[REDACTED SLACK WEBHOOK]")
	redacted = privateKeyPattern.ReplaceAllString(redacted, "[REDACTED PRIVATE KEY]")
	redacted = credentialURLPattern.ReplaceAllString(redacted, "${1}"+Redacted+"@")
	redacted = bearerPattern.ReplaceAllString(redacted, "Bearer "+Redacted)
	redacted = basicAuthPattern.ReplaceAllString(redacted, "Basic "+Redacted)
	redacted = githubTokenPattern.ReplaceAllString(redacted, "[REDACTED GITHUB TOKEN]")
	redacted = openAITokenPattern.ReplaceAllString(redacted, "[REDACTED OPENAI TOKEN]")
	redacted = awsAccessKeyPattern.ReplaceAllString(redacted, "[REDACTED AWS ACCESS KEY]")
	redacted = jwtPattern.ReplaceAllString(redacted, "[REDACTED JWT]")
	redacted = keyValuePattern.ReplaceAllString(redacted, "${1}"+Redacted)
	redacted = longTokenPattern.ReplaceAllString(redacted, "[REDACTED TOKEN]")
	return redactSuspiciousEncodedLines(redacted)
}

func redactSuspiciousEncodedLines(value string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		if encodedSlackHint.MatchString(line) || encodedCredentialHint.MatchString(line) || escapedCredentialHint.MatchString(line) {
			lines[index] = "[REDACTED CREDENTIAL-BEARING LINE]"
		}
	}
	return strings.Join(lines, "\n")
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
