package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadClaude(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.json")
	if err := os.WriteFile(path, []byte(`{
	  "oauthAccount": {
	    "emailAddress": "a@example.com",
	    "organizationType": "claude_max",
	    "organizationRateLimitTier": "default_claude_max_20x"
	  }
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	a := ReadClaude(path)
	if a.Email != "a@example.com" {
		t.Fatalf("email %q", a.Email)
	}
	if a.Plan != "claude_max default_claude_max_20x" {
		t.Fatalf("plan %q", a.Plan)
	}
	if a.Err != "" {
		t.Fatalf("err %q", a.Err)
	}
}

func TestReadClaudeMissing(t *testing.T) {
	a := ReadClaude(filepath.Join(t.TempDir(), "nope"))
	if a.Err == "" {
		t.Fatal("expected error")
	}
}

func TestReadCodex(t *testing.T) {
	// header.payload.sig — payload is {"email":"b@example.com","https://api.openai.com/auth":{"chatgpt_plan_type":"pro"}}
	payload := "eyJlbWFpbCI6ImJAZXhhbXBsZS5jb20iLCJodHRwczovL2FwaS5vcGVuYWkuY29tL2F1dGgiOnsiY2hhdGdwdF9wbGFuX3R5cGUiOiJwcm8ifX0"
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	body := `{"auth_mode":"chatgpt","tokens":{"id_token":"aaa.` + payload + `.sig"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	a := ReadCodex(path)
	if a.Email != "b@example.com" {
		t.Fatalf("email %q", a.Email)
	}
	if a.Plan != "pro" {
		t.Fatalf("plan %q", a.Plan)
	}
}

func TestReadGrok(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	body := `{
	  "https://auth.x.ai::abc": {
	    "email": "c@example.com",
	    "auth_mode": "oidc",
	    "key": "eyJhbGciOiJub25lIn0.eyJ0aWVyIjo1fQ.x"
	  }
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	a := ReadGrok(path)
	if a.Email != "c@example.com" {
		t.Fatalf("email %q", a.Email)
	}
	if a.Plan != "oidc tier5" {
		t.Fatalf("plan %q", a.Plan)
	}
}
