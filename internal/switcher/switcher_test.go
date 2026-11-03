package switcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndUseLocal(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, ".claude.json")
	creds := filepath.Join(home, ".claude", ".credentials.json")
	if err := os.MkdirAll(filepath.Dir(creds), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claude, []byte(`{"oauthAccount":{"emailAddress":"a@x.ai","organizationType":"claude_max"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(creds, []byte(`{"claudeAiOauth":{"accessToken":"access-token-value-ok","refreshToken":"refresh-token-value-ok"},"mcpOAuth":{"keep":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := SaveLocal(home, "work", "test")
	if err != nil {
		t.Fatal(err)
	}
	if m.Email != "a@x.ai" {
		t.Fatalf("email %q", m.Email)
	}

	// Change live login to something else, then switch back.
	if err := os.WriteFile(claude, []byte(`{"oauthAccount":{"emailAddress":"b@y.ai"},"other":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(creds, []byte(`{"claudeAiOauth":{"accessToken":"tok-b"},"mcpOAuth":{"keep":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := UseLocal(home, "work"); err != nil {
		t.Fatal(err)
	}
	var oa struct {
		OAuthAccount struct {
			Email string `json:"emailAddress"`
		} `json:"oauthAccount"`
		Other int `json:"other"`
	}
	raw, _ := os.ReadFile(claude)
	if err := json.Unmarshal(raw, &oa); err != nil {
		t.Fatal(err)
	}
	if oa.OAuthAccount.Email != "a@x.ai" {
		t.Fatalf("oauth %q", oa.OAuthAccount.Email)
	}
	if oa.Other != 1 {
		t.Fatal("must preserve other claude.json fields")
	}
	var cred map[string]any
	_ = json.Unmarshal(mustRead(t, creds), &cred)
	blob := cred["claudeAiOauth"].(map[string]any)
	if blob["accessToken"] != "access-token-value-ok" {
		t.Fatalf("token %v", blob["accessToken"])
	}
	if cred["mcpOAuth"] == nil {
		t.Fatal("must preserve mcpOAuth")
	}
}

func TestUsableBlobRejectsStub(t *testing.T) {
	if err := usableBlob(map[string]any{"accessToken": "short", "refreshToken": "x"}); err == nil {
		t.Fatal("expected reject")
	}
	if err := usableBlob(map[string]any{"accessToken": "tok-a-not-real-but-20c", "refreshToken": "refresh-token-is-long-enough"}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateName(t *testing.T) {
	if err := ValidateName("../etc"); err == nil {
		t.Fatal("expected error")
	}
	if err := ValidateName("gmail"); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
