package usage

import (
	"encoding/json"
	"testing"
)

func TestAppendClaudeWindows(t *testing.T) {
	body := map[string]any{
		"five_hour": map[string]any{"utilization": 12.5, "resets_at": "2026-08-15T20:00:00Z"},
		"seven_day": map[string]any{"utilization": 40.0},
	}
	var w []Window
	w = appendWindow(w, body, "five_hour", "5h")
	w = appendWindow(w, body, "seven_day", "wk")
	if len(w) != 2 || w[0].Display != "12%" || w[1].Display != "40%" {
		t.Fatalf("%+v", w)
	}
}

func TestAppendCodexWindowsUsedPercent(t *testing.T) {
	rl := map[string]any{
		"primary_window":   map[string]any{"used_percent": 81.0},
		"secondary_window": map[string]any{"used": 2.0, "limit": 10.0},
	}
	var w []Window
	w = appendCodexWindow(w, rl, "primary_window", "5h")
	w = appendCodexWindow(w, rl, "secondary_window", "wk")
	if len(w) != 2 {
		t.Fatalf("len %d", len(w))
	}
	if w[0].Display != "81%" {
		t.Fatalf("primary %q", w[0].Display)
	}
	if w[1].Display != "20%" {
		t.Fatalf("secondary %q", w[1].Display)
	}
}

func TestFmtPctFraction(t *testing.T) {
	if g := fmtPct(0.62); g != "62%" {
		t.Fatalf("%s", g)
	}
}

func TestParseGrokBillingFixture(t *testing.T) {
	raw := []byte(`{"config":{"creditUsagePercent":33.3,"subscription_tier":"super"}}`)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	cfg := body["config"].(map[string]any)
	p := floatPtr(cfg, "creditUsagePercent")
	if p == nil || *p < 33 || *p > 34 {
		t.Fatalf("%v", p)
	}
}

func TestCodexWindowLabelWeekly(t *testing.T) {
	if g := windowLabel(ptr(604800)); g != "wk" {
		t.Fatalf("%s", g)
	}
	if g := windowLabel(ptr(18000)); g != "5h" {
		t.Fatalf("%s", g)
	}
}

func ptr(f float64) *float64 { return &f }
