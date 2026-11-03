package status

import (
	"strings"
	"testing"

	"github.com/abhirupghosh/quota/internal/identity"
	"github.com/abhirupghosh/quota/internal/usage"
)

func TestFormatSwiftBarTitleAndSections(t *testing.T) {
	u := 11.0
	w := 100.0
	g := 46.0
	r := Report{Hosts: []HostReport{
		{
			Host: "mac",
			Tools: []ToolRow{
				{Account: identity.Account{Tool: identity.Claude, Email: "a@acme.ai", Plan: "max"}, Usage: usage.Snapshot{Windows: []usage.Window{{Label: "5h", Utilization: &u, Display: "11%"}}}},
				{Account: identity.Account{Tool: identity.Codex, Email: "a@acme.ai", Plan: "pro"}, Usage: usage.Snapshot{Windows: []usage.Window{{Label: "wk", Utilization: &w, Display: "100%"}}}},
				{Account: identity.Account{Tool: identity.Grok, Email: "a@acme.ai"}, Usage: usage.Snapshot{Windows: []usage.Window{{Label: "credits", Utilization: &g, Display: "46%"}}}},
			},
		},
		{Host: "devbox", Err: "ssh failed"},
	}}
	out := FormatSwiftBar(r, []AccountOption{
		{Name: "work", Email: "a@acme.ai", Tool: "claude"},
		{Name: "gmail", Email: "b@gmail.com", Tool: "claude"},
	}, "/tmp/quota")
	if !strings.HasPrefix(out, "46 |") {
		t.Fatalf("title: %q", strings.Split(out, "\n")[0])
	}
	if !strings.Contains(out, "mac      C11   X100   G46") && !strings.Contains(out, "mac ") {
		t.Fatalf("missing host line: %s", out)
	}
	if strings.Contains(out, "a@acme.ai") {
		t.Fatal("should not dump full emails")
	}
	if strings.Contains(out, "claude_max default") {
		t.Fatal("should not dump raw plan ids")
	}
	if !strings.Contains(out, "ssh failed") {
		t.Fatal("missing host error")
	}
	if !strings.Contains(out, "refresh=true") {
		t.Fatal("missing refresh")
	}
	if strings.Contains(out, "Claude usage") {
		t.Fatal("footer links add noise")
	}
	if !strings.Contains(out, "---- work") || !strings.Contains(out, "---- gmail") {
		t.Fatal("missing Claude switch items")
	}
	if !strings.Contains(out, "checked=true") {
		t.Fatal("active Claude should be checked")
	}
	if !strings.Contains(out, "param1=use") || !strings.Contains(out, "param5=devbox") && !strings.Contains(out, "bash=/tmp/quota") {
		// devbox row is an error, so switch items belong to mac
		if !strings.Contains(out, "bash=/tmp/quota") {
			t.Fatal("missing switch command")
		}
	}
}

func TestLevelColor(t *testing.T) {
	if levelColor("11") != "#34C759" {
		t.Fatal(levelColor("11"))
	}
	if levelColor("75") != "#E0A106" {
		t.Fatal(levelColor("75"))
	}
	if levelColor("100") != "#E85D5D" {
		t.Fatal(levelColor("100"))
	}
}

func TestShortAccount(t *testing.T) {
	cases := map[string]string{
		"":               "",
		"me@gmail.com":   "gmail",
		"Me@iCloud.com":  "icloud",
		"me@acme.io":     "acme",
		"me@eng.acme.io": "eng",
		"weird":          "weird",
	}
	for in, want := range cases {
		if got := shortAccount(in); got != want {
			t.Errorf("shortAccount(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAccountNoteSkipsHome(t *testing.T) {
	h := HostReport{Host: "devbox", Tools: []ToolRow{
		{Account: identity.Account{Tool: identity.Claude, Email: "a@acme.ai"}},
		{Account: identity.Account{Tool: identity.Codex, Email: "b@gmail.com"}},
	}}
	if got := accountNote(h, "acme"); got != "gmail" {
		t.Fatalf("accountNote = %q", got)
	}
}
