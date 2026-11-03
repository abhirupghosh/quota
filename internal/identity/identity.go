package identity

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
)

type Tool string

const (
	Claude Tool = "claude"
	Codex  Tool = "codex"
	Grok   Tool = "grok"
)

type Account struct {
	Tool  Tool   `json:"tool"`
	Email string `json:"email,omitempty"`
	Plan  string `json:"plan,omitempty"`
	Extra string `json:"extra,omitempty"`
	Err   string `json:"error,omitempty"`
}

func (a Account) Key() string {
	return string(a.Tool) + "|" + strings.ToLower(strings.TrimSpace(a.Email))
}

func ReadClaude(path string) Account {
	a := Account{Tool: Claude}
	raw, err := os.ReadFile(path)
	if err != nil {
		a.Err = "no ~/.claude.json"
		return a
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		a.Err = "claude.json: bad json"
		return a
	}
	oa, _ := root["oauthAccount"].(map[string]any)
	if oa == nil {
		a.Err = "not logged in"
		return a
	}
	a.Email, _ = oa["emailAddress"].(string)
	plan, _ := oa["organizationType"].(string)
	tier, _ := oa["organizationRateLimitTier"].(string)
	a.Plan = strings.TrimSpace(plan)
	if tier != "" {
		if a.Plan != "" {
			a.Plan += " "
		}
		a.Plan += tier
	}
	if a.Email == "" {
		a.Err = "no email in oauthAccount"
	}
	return a
}

func ReadCodex(path string) Account {
	a := Account{Tool: Codex}
	raw, err := os.ReadFile(path)
	if err != nil {
		a.Err = "no ~/.codex/auth.json"
		return a
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		a.Err = "codex auth: bad json"
		return a
	}
	tokens, _ := root["tokens"].(map[string]any)
	if tokens == nil {
		a.Err = "not logged in"
		return a
	}
	idTok, _ := tokens["id_token"].(string)
	claims := jwtClaims(idTok)
	if claims == nil {
		a.Err = "id_token unreadable"
		return a
	}
	a.Email = firstString(claims, "email", "https://api.openai.com/profile")
	if a.Email == "" {
		if prof, ok := claims["https://api.openai.com/profile"].(map[string]any); ok {
			a.Email, _ = prof["email"].(string)
		}
	}
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		a.Plan, _ = auth["chatgpt_plan_type"].(string)
	}
	if a.Email == "" {
		a.Err = "no email in token"
	}
	return a
}

func ReadGrok(path string) Account {
	a := Account{Tool: Grok}
	raw, err := os.ReadFile(path)
	if err != nil {
		a.Err = "no ~/.grok/auth.json"
		return a
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		a.Err = "grok auth: bad json"
		return a
	}
	for _, v := range root {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		a.Email, _ = entry["email"].(string)
		if a.Email == "" {
			continue
		}
		if mode, _ := entry["auth_mode"].(string); mode != "" {
			a.Plan = mode
		}
		if team, _ := entry["team_id"].(string); team != "" {
			a.Extra = "team"
		}
		if key, _ := entry["key"].(string); key != "" {
			if claims := jwtClaims(key); claims != nil {
				if tier, ok := claims["tier"].(float64); ok {
					if a.Plan != "" {
						a.Plan += " "
					}
					a.Plan += "tier" + trimFloat(tier)
				}
			}
		}
		return a
	}
	a.Err = "not logged in"
	return a
}

func jwtClaims(tok string) map[string]any {
	parts := strings.Split(tok, ".")
	if len(parts) < 2 {
		return nil
	}
	payload := parts[1]
	if m := len(payload) % 4; m != 0 {
		payload += strings.Repeat("=", 4-m)
	}
	b, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		b, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil
		}
	}
	var claims map[string]any
	if json.Unmarshal(b, &claims) != nil {
		return nil
	}
	return claims
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func trimFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(strings.ReplaceAll(formatFloat(f), " ", ""), "0"), ".")
}

func formatFloat(f float64) string {
	return strings.TrimSpace(strings.Replace(strings.Replace(jsonNumber(f), "e+", "e", 1), ".0", "", 1))
}

func jsonNumber(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}
