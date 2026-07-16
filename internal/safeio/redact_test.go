package safeio

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactStringRemovesSecretLikeValues(t *testing.T) {
	raw := strings.Join([]string{
		"api_key=abc123",
		"token: supertoken",
		"password=hunter2",
		"Authorization: Bearer abcdef123456",
		"https://hooks.slack.com/services/T000/B000/slack-secret",
		"pk_fixture_123456789012345678901234",
		"-----BEGIN PRIVATE KEY-----\nkey material\n-----END PRIVATE KEY-----",
	}, "\n")

	redacted := RedactString(raw)
	for _, forbidden := range []string{"abc123", "supertoken", "hunter2", "abcdef123456", "slack-secret", "pk_fixture_123456789012345678901234", "key material"} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("redacted string contains %q: %s", forbidden, redacted)
		}
	}
	for _, marker := range []string{Redacted, "[REDACTED SLACK WEBHOOK]", "[REDACTED TOKEN]", "[REDACTED PRIVATE KEY]"} {
		if !strings.Contains(redacted, marker) {
			t.Fatalf("redacted string missing marker %q: %s", marker, redacted)
		}
	}
}

func TestRedactStringCoversCommonCredentialFamilies(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		secret string
	}{
		{name: "github classic", input: "used ghp_1234567890abcdefghijklmnopqrstuvwxyz", secret: "ghp_1234567890abcdefghijklmnopqrstuvwxyz"},
		{name: "github fine grained", input: "used github_pat_11AA22BB33CC44DD55EE66FF", secret: "github_pat_11AA22BB33CC44DD55EE66FF"},
		{name: "openai project", input: "used sk-proj-1234567890abcdefghijklmnop", secret: "sk-proj-1234567890abcdefghijklmnop"},
		{name: "openai legacy", input: "used sk-1234567890abcdefghijklmnop", secret: "sk-1234567890abcdefghijklmnop"},
		{name: "aws access key", input: "used AKIAIOSFODNN7EXAMPLE", secret: "AKIAIOSFODNN7EXAMPLE"},
		{name: "jwt", input: "used eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature123", secret: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature123"},
		{name: "basic auth", input: "Authorization: Basic dXNlcjpwYXNzd29yZA==", secret: "dXNlcjpwYXNzd29yZA=="},
		{name: "cookie", input: "Cookie: session_id=cookie-secret; csrf=csrf-secret", secret: "cookie-secret"},
		{name: "session", input: "session_token=session-secret", secret: "session-secret"},
		{name: "credential url", input: "postgres://database-user:database-password@example.test/app", secret: "database-password"},
		{name: "escaped credential url", input: `https:\/\/escaped-user:escaped-password@example.test/path`, secret: "escaped-password"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			redacted := RedactString(test.input)
			if strings.Contains(redacted, test.secret) {
				t.Fatalf("redacted string contains credential %q: %s", test.secret, redacted)
			}
			if !strings.Contains(redacted, Redacted) && !strings.Contains(redacted, "[REDACTED ") {
				t.Fatalf("redacted string is missing a marker: %s", redacted)
			}
		})
	}
}

func TestRedactStringFailsClosedForEncodedOrEscapedCredentialLines(t *testing.T) {
	raw := strings.Join([]string{
		"safe line remains",
		"Authorization%3A%20Bearer%20percent-secret-123456",
		"https%3A%2F%2Fuser%3Aencoded-password%40example.test%2Fpath",
		"https%3A%2F%2Fhooks.slack.com%2Fservices%2FT000%2FB000%2Fencoded-slack-secret",
		"github%5Fpat%5Fencoded-token-1234567890",
		`{\"token\":\"escaped-secret-value\"}`,
	}, "\n")

	redacted := RedactString(raw)
	if !strings.Contains(redacted, "safe line remains") {
		t.Fatalf("safe line should remain: %s", redacted)
	}
	for _, forbidden := range []string{"percent-secret", "encoded-password", "encoded-slack-secret", "encoded-token", "escaped-secret"} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("encoded credential line contains %q: %s", forbidden, redacted)
		}
	}
	if count := strings.Count(redacted, "[REDACTED CREDENTIAL-BEARING LINE]"); count != 5 {
		t.Fatalf("expected 5 fail-closed lines, got %d: %s", count, redacted)
	}
}

func TestRedactStringNeverConsumesTheFollowingLine(t *testing.T) {
	input := strings.Join([]string{
		"- Work performed: Authorization:",
		"- Outcome: kept after redaction",
		"- Note: Bearer",
		"- Learning: also kept",
		"token: actual-secret",
		"- Verification: still present",
	}, "\n")

	redacted := RedactString(input)
	for _, expected := range []string{
		"- Work performed: Authorization:",
		"- Outcome: kept after redaction",
		"- Note: Bearer",
		"- Learning: also kept",
		"token: " + Redacted,
		"- Verification: still present",
	} {
		if !strings.Contains(redacted, expected) {
			t.Fatalf("expected %q in redacted output:\n%s", expected, redacted)
		}
	}
	if strings.Contains(redacted, "actual-secret") {
		t.Fatalf("secret was not redacted:\n%s", redacted)
	}
}

func TestRedactValueRemovesNestedSecretLikeValues(t *testing.T) {
	value := map[string]any{
		"message": "safe text with Bearer nestedtoken123",
		"delivery": map[string]any{
			"slack_webhook_url": "https://hooks.slack.com/services/T000/B000/value-secret",
			"configured":        true,
		},
		"events": []any{
			map[string]any{"password": "hunter2"},
			"api_key=abc123",
		},
		"private_key": "-----BEGIN PRIVATE KEY-----\nmaterial\n-----END PRIVATE KEY-----",
	}

	redacted := RedactValue(value)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	output := string(encoded)
	for _, forbidden := range []string{"nestedtoken123", "value-secret", "hunter2", "abc123", "material"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("redacted value contains %q: %s", forbidden, output)
		}
	}
	if !strings.Contains(output, Redacted) {
		t.Fatalf("redacted value missing marker: %s", output)
	}
}

func TestRedactJSONRemovesSecretLikeValuesButConfigStorageMayRemainRaw(t *testing.T) {
	rawConfig := []byte(`{
		"delivery": {"slack_webhook_url": "https://hooks.slack.com/services/T000/B000/config-secret"},
		"defaults": {"output_dir": "/tmp/plain"},
		"session": {"password": "hunter2", "note": "Bearer abcdef123456 api_key=abc123"}
	}`)
	if !strings.Contains(string(rawConfig), "config-secret") {
		t.Fatal("test fixture should document raw config storage can contain the secret")
	}

	redacted, err := RedactJSON(rawConfig)
	if err != nil {
		t.Fatal(err)
	}
	output := string(redacted)
	for _, forbidden := range []string{"config-secret", "hunter2", "abcdef123456", "abc123"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("redacted JSON contains %q: %s", forbidden, output)
		}
	}
	if !strings.Contains(output, Redacted) {
		t.Fatalf("redacted JSON missing marker: %s", output)
	}
}

func TestIsSensitiveKeyCoversWebhookAndTokenFamilies(t *testing.T) {
	for _, key := range []string{"password", "api_key", "access-key", "private_key", "slack_webhook_url", "refresh_token", "client-secret", "authorization", "set_cookie", "session_id", "credentials", "csrf_token"} {
		if !IsSensitiveKey(key) {
			t.Fatalf("%q should be sensitive", key)
		}
	}
	for _, key := range []string{"message", "configured", "output_dir", "target_model", "session_count", "slack_webhook_configured"} {
		if IsSensitiveKey(key) {
			t.Fatalf("%q should not be sensitive", key)
		}
	}
}
