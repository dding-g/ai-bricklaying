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
	for _, key := range []string{"password", "api_key", "access-key", "private_key", "slack_webhook_url", "refresh_token", "client-secret"} {
		if !IsSensitiveKey(key) {
			t.Fatalf("%q should be sensitive", key)
		}
	}
	for _, key := range []string{"message", "configured", "output_dir", "target_model"} {
		if IsSensitiveKey(key) {
			t.Fatalf("%q should not be sensitive", key)
		}
	}
}
