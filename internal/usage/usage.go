package usage

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/abhirupghosh/quota/internal/config"
	"github.com/abhirupghosh/quota/internal/identity"
)

type Window struct {
	Label       string   `json:"label"`
	Utilization *float64 `json:"utilization,omitempty"`
	ResetsAt    string   `json:"resets_at,omitempty"`
	Display     string   `json:"display,omitempty"`
}

type Snapshot struct {
	Tool    identity.Tool `json:"tool"`
	Account string        `json:"account,omitempty"`
	Windows []Window      `json:"windows,omitempty"`
	Err     string        `json:"error,omitempty"`
	Stale   bool          `json:"stale,omitempty"`
}

func Fetch(tool identity.Tool, home string) Snapshot {
	switch tool {
	case identity.Claude:
		return fetchClaude(home)
	case identity.Codex:
		return fetchCodex(home)
	case identity.Grok:
		return fetchGrok(home)
	default:
		return Snapshot{Tool: tool, Err: "unknown tool"}
	}
}

func fetchClaude(home string) Snapshot {
	s := Snapshot{Tool: identity.Claude}
	tok := claudeAccessToken(home)
	if tok == "" {
		s.Err = "no claude access token"
		return s
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.anthropic.com/api/oauth/usage", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "quota/0.1 (cli)")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	body, err := doJSON(req)
	if err != nil {
		s.Err = err.Error()
		return s
	}
	s.Windows = appendWindow(s.Windows, body, "five_hour", "5h")
	s.Windows = appendWindow(s.Windows, body, "seven_day", "wk")
	if len(s.Windows) == 0 {
		s.Err = "claude usage: no windows"
	}
	return s
}

func fetchCodex(home string) Snapshot {
	s := Snapshot{Tool: identity.Codex}
	raw, err := os.ReadFile(config.CodexAuth(home))
	if err != nil {
		s.Err = "no ~/.codex/auth.json"
		return s
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		s.Err = "codex auth: bad json"
		return s
	}
	tokens, _ := root["tokens"].(map[string]any)
	if tokens == nil {
		s.Err = "not logged in"
		return s
	}
	tok, _ := tokens["access_token"].(string)
	if tok == "" {
		s.Err = "no access_token"
		return s
	}
	req, _ := http.NewRequest(http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "quota/0.1 (cli)")
	body, err := doJSON(req)
	if err != nil {
		s.Err = err.Error()
		return s
	}
	// shape: rate_limit.primary_window / secondary_window with used_percent or similar
	rl, _ := body["rate_limit"].(map[string]any)
	if rl == nil {
		rl = body
	}
	s.Windows = appendCodexWindow(s.Windows, rl, "primary_window", "")
	s.Windows = appendCodexWindow(s.Windows, rl, "secondary_window", "")
	if extras, ok := body["additional_rate_limits"].([]any); ok {
		for _, raw := range extras {
			ex, _ := raw.(map[string]any)
			if ex == nil {
				continue
			}
			name, _ := ex["limit_name"].(string)
			erl, _ := ex["rate_limit"].(map[string]any)
			if erl == nil {
				continue
			}
			before := len(s.Windows)
			s.Windows = appendCodexWindow(s.Windows, erl, "primary_window", name)
			if len(s.Windows) == before {
				s.Windows = appendCodexWindow(s.Windows, erl, "secondary_window", name)
			}
		}
	}
	if len(s.Windows) == 0 {
		s.Err = "codex usage: no windows"
	}
	return s
}

func fetchGrok(home string) Snapshot {
	s := Snapshot{Tool: identity.Grok}
	tok := grokAccessToken(home)
	if tok == "" {
		s.Err = "no grok access token"
		return s
	}
	req, _ := http.NewRequest(http.MethodGet, "https://cli-chat-proxy.grok.com/v1/billing?format=credits", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "quota/0.1 (cli)")
	req.Header.Set("x-grok-client-mode", "cli")
	body, err := doJSON(req)
	if err != nil {
		s.Err = err.Error()
		return s
	}
	cfg, _ := body["config"].(map[string]any)
	if cfg == nil {
		cfg = body
	}
	if p := floatPtr(cfg, "creditUsagePercent"); p != nil {
		s.Windows = append(s.Windows, Window{Label: "credits", Utilization: p, Display: fmtPct(*p)})
	}
	if p := floatPtr(cfg, "utilization"); p != nil && len(s.Windows) == 0 {
		s.Windows = append(s.Windows, Window{Label: "period", Utilization: p, Display: fmtPct(*p)})
	}
	if products, ok := cfg["productUsage"].([]any); ok {
		for _, raw := range products {
			prod, _ := raw.(map[string]any)
			if prod == nil {
				continue
			}
			name, _ := prod["product"].(string)
			if name == "" {
				name = "product"
			}
			if p := floatPtr(prod, "usagePercent"); p != nil {
				// skip duplicate of the top-level credits percent
				if len(s.Windows) == 1 && s.Windows[0].Utilization != nil && almostEq(*s.Windows[0].Utilization, *p) {
					continue
				}
				s.Windows = append(s.Windows, Window{Label: name, Utilization: p, Display: fmtPct(*p)})
			}
		}
	}
	if tier, _ := cfg["subscription_tier"].(string); tier != "" {
		s.Windows = append(s.Windows, Window{Label: "plan", Display: tier})
	}
	if len(s.Windows) == 0 {
		s.Err = "grok billing: no usage fields"
	}
	return s
}

func claudeAccessToken(home string) string {
	path := config.ClaudeCreds(home)
	if raw, err := os.ReadFile(path); err == nil {
		var root map[string]any
		if json.Unmarshal(raw, &root) == nil {
			if oauth, ok := root["claudeAiOauth"].(map[string]any); ok {
				if t, _ := oauth["accessToken"].(string); t != "" {
					return t
				}
			}
		}
	}
	// macOS Keychain — common Claude Code service names
	for _, svc := range []string{"Claude Code-credentials", "Claude Code"} {
		if t := keychainJSONToken(svc); t != "" {
			return t
		}
	}
	return ""
}

func keychainJSONToken(service string) string {
	out, err := exec.Command("security", "find-generic-password", "-s", service, "-w").Output()
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return ""
	}
	var root map[string]any
	if json.Unmarshal([]byte(s), &root) != nil {
		return ""
	}
	if oauth, ok := root["claudeAiOauth"].(map[string]any); ok {
		if t, _ := oauth["accessToken"].(string); t != "" {
			return t
		}
	}
	if t, _ := root["accessToken"].(string); t != "" {
		return t
	}
	return ""
}

func grokAccessToken(home string) string {
	raw, err := os.ReadFile(config.GrokAuth(home))
	if err != nil {
		return ""
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return ""
	}
	for _, v := range root {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := entry["key"].(string); t != "" {
			return t
		}
	}
	return ""
}

func doJSON(req *http.Request) (map[string]any, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 160 {
			msg = msg[:160]
		}
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.Unmarshal(b, &body); err != nil {
		return nil, fmt.Errorf("bad json")
	}
	return body, nil
}

func appendWindow(dst []Window, body map[string]any, key, label string) []Window {
	w, _ := body[key].(map[string]any)
	if w == nil {
		return dst
	}
	u := floatPtr(w, "utilization")
	reset, _ := w["resets_at"].(string)
	if u == nil && reset == "" {
		return dst
	}
	win := Window{Label: label, Utilization: u, ResetsAt: reset}
	if u != nil {
		win.Display = fmtPct(*u)
	}
	return append(dst, win)
}

func appendCodexWindow(dst []Window, rl map[string]any, key, label string) []Window {
	w, _ := rl[key].(map[string]any)
	if w == nil {
		return dst
	}
	u := floatPtr(w, "used_percent")
	if u == nil {
		u = floatPtr(w, "utilization")
	}
	if u == nil {
		used := floatPtr(w, "used")
		limit := floatPtr(w, "limit")
		if used != nil && limit != nil && *limit > 0 {
			v := 100 * (*used) / (*limit)
			u = &v
		}
	}
	reset, _ := w["reset_at"].(string)
	if reset == "" {
		reset, _ = w["resets_at"].(string)
	}
	if u == nil && reset == "" {
		return dst
	}
	if label == "" {
		label = windowLabel(floatPtr(w, "limit_window_seconds"))
	}
	win := Window{Label: label, Utilization: u, ResetsAt: reset}
	if u != nil {
		win.Display = fmtPct(*u)
	}
	return append(dst, win)
}

func windowLabel(secs *float64) string {
	if secs == nil {
		return "win"
	}
	switch {
	case *secs >= 600000 && *secs <= 620000: // ~7d
		return "wk"
	case *secs >= 17000 && *secs <= 19000: // ~5h
		return "5h"
	case *secs >= 3500 && *secs <= 3700:
		return "1h"
	default:
		h := *secs / 3600
		if h >= 20 {
			return fmt.Sprintf("%.0fd", h/24)
		}
		return fmt.Sprintf("%.0fh", h)
	}
}

func almostEq(a, b float64) bool {
	if a > b {
		a, b = b, a
	}
	return b-a < 0.51
}

func floatPtr(m map[string]any, key string) *float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		return &n
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

func fmtPct(v float64) string {
	if v > 0 && v <= 1 {
		v *= 100
	}
	return fmt.Sprintf("%.0f%%", v)
}
