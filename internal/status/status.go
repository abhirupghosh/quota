package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/abhirupghosh/quota/internal/config"
	"github.com/abhirupghosh/quota/internal/identity"
	"github.com/abhirupghosh/quota/internal/usage"
)

type ToolRow struct {
	identity.Account
	Usage usage.Snapshot `json:"usage"`
}

type HostReport struct {
	Host  string    `json:"host"`
	SSH   string    `json:"ssh,omitempty"`
	Err   string    `json:"error,omitempty"`
	Tools []ToolRow `json:"tools"`
}

type Report struct {
	Generated time.Time    `json:"generated"`
	Hosts     []HostReport `json:"hosts"`
}

func Collect(hosts []config.Host, only string) Report {
	rep := Report{Generated: time.Now()}
	var localHome = config.Home()
	var localIdent = map[identity.Tool]identity.Account{
		identity.Claude: identity.ReadClaude(config.ClaudeJSON(localHome)),
		identity.Codex:  identity.ReadCodex(config.CodexAuth(localHome)),
		identity.Grok:   identity.ReadGrok(config.GrokAuth(localHome)),
	}
	reports := make([]HostReport, len(hosts))
	var wg sync.WaitGroup
	for i, h := range hosts {
		wg.Add(1)
		go func(i int, h config.Host) {
			defer wg.Done()
			reports[i] = probeHost(h)
		}(i, h)
	}
	wg.Wait()

	// Per-account fetch. If this Mac's token is 429/401, use a remote
	// that is logged into the same account (another host may still hold a working token).
	remotes := map[string][]string{}
	seen := map[string]identity.Account{}
	for _, hr := range reports {
		for _, t := range hr.Tools {
			if t.Email == "" {
				continue
			}
			k := t.Key()
			seen[k] = t.Account
			if hr.SSH != "" {
				remotes[k] = append(remotes[k], hr.SSH)
			}
		}
	}
	snaps := map[string]usage.Snapshot{}
	var mu sync.Mutex
	for k, acct := range seen {
		wg.Add(1)
		go func(k string, acct identity.Account) {
			defer wg.Done()
			snap := fetchAccount(acct, localIdent[acct.Tool], localHome, remotes[k])
			mu.Lock()
			snaps[k] = snap
			mu.Unlock()
		}(k, acct)
	}
	wg.Wait()
	for i := range reports {
		for j := range reports[i].Tools {
			k := reports[i].Tools[j].Key()
			if s, ok := snaps[k]; ok {
				reports[i].Tools[j].Usage = s
			}
		}
	}
	if only != "" {
		var filtered []HostReport
		for _, hr := range reports {
			if hr.Host == only || hr.SSH == only {
				filtered = append(filtered, hr)
			}
		}
		rep.Hosts = filtered
	} else {
		rep.Hosts = reports
	}
	return rep
}

func fetchAccount(acct, local identity.Account, localHome string, sshHosts []string) usage.Snapshot {
	if acct.Email == "" || acct.Err != "" {
		return usage.Snapshot{Tool: acct.Tool, Err: acct.Err}
	}
	usable := func(s usage.Snapshot) bool {
		return len(s.Windows) > 0
	}
	if local.Email != "" && strings.EqualFold(local.Email, acct.Email) {
		snap := usage.CachedFetch(acct.Tool, localHome, acct.Email)
		if usable(snap) && snap.Err == "" {
			return snap
		}
		if usable(snap) && !strings.Contains(snap.Err, "401") && !strings.Contains(snap.Err, "429") {
			return snap
		}
		// local token dead — try remotes before giving up on a 429 with no windows
		for _, sshHost := range sshHosts {
			remote := fetchRemoteUsage(sshHost, acct)
			if usable(remote) {
				usage.Remember(remote)
				return remote
			}
		}
		if usable(snap) {
			return snap
		}
		if stale, ok := usage.Recall(acct.Tool, acct.Email); ok {
			stale.Stale = true
			return stale
		}
		return snap
	}
	if stale, ok := usage.Recall(acct.Tool, acct.Email); ok && !stale.Stale {
		return stale
	}
	for _, sshHost := range sshHosts {
		remote := fetchRemoteUsage(sshHost, acct)
		if usable(remote) {
			usage.Remember(remote)
			return remote
		}
	}
	if stale, ok := usage.Recall(acct.Tool, acct.Email); ok {
		stale.Stale = true
		return stale
	}
	if len(sshHosts) == 0 {
		return usage.Snapshot{Tool: acct.Tool, Account: acct.Email, Err: "no local token for this account"}
	}
	return usage.Snapshot{Tool: acct.Tool, Account: acct.Email, Err: "usage unavailable"}
}

func probeHost(h config.Host) HostReport {
	hr := HostReport{Host: h.Name, SSH: h.SSH}
	if h.Local || h.SSH == "" {
		home := config.Home()
		hr.Tools = []ToolRow{
			{Account: identity.ReadClaude(config.ClaudeJSON(home))},
			{Account: identity.ReadCodex(config.CodexAuth(home))},
			{Account: identity.ReadGrok(config.GrokAuth(home))},
		}
		return hr
	}
	cmd := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", h.SSH, "python3", "-")
	cmd.Stdin = strings.NewReader(remoteIdentityPy)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		hr.Err = "ssh: " + compact(msg)
		return hr
	}
	var got struct {
		Claude identity.Account `json:"claude"`
		Codex  identity.Account `json:"codex"`
		Grok   identity.Account `json:"grok"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		hr.Err = "remote identity: bad json"
		return hr
	}
	if got.Claude.Tool == "" {
		got.Claude.Tool = identity.Claude
	}
	if got.Codex.Tool == "" {
		got.Codex.Tool = identity.Codex
	}
	if got.Grok.Tool == "" {
		got.Grok.Tool = identity.Grok
	}
	hr.Tools = []ToolRow{{Account: got.Claude}, {Account: got.Codex}, {Account: got.Grok}}
	return hr
}

func fetchRemoteUsage(sshHost string, acct identity.Account) usage.Snapshot {
	s := usage.Snapshot{Tool: acct.Tool, Account: acct.Email}
	cmd := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", sshHost, "python3", "-", string(acct.Tool))
	cmd.Stdin = strings.NewReader(remoteUsagePy)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		s.Err = "remote usage: " + compact(msg)
		return s
	}
	var got struct {
		Windows []usage.Window `json:"windows"`
		Err     string         `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		s.Err = "remote usage: bad json"
		return s
	}
	s.Windows = got.Windows
	s.Err = got.Err
	return s
}

func FormatTable(r Report) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "HOST\tTOOL\tACCOUNT\tPLAN\tUSAGE")
	for _, h := range r.Hosts {
		if h.Err != "" {
			fmt.Fprintf(w, "%s\t-\t-\t-\t%s\n", h.Host, h.Err)
			continue
		}
		for _, t := range h.Tools {
			acct := t.Email
			if acct == "" {
				acct = "—"
			}
			plan := t.Plan
			if plan == "" {
				plan = "—"
			}
			use := usageLine(t)
			if t.Err != "" && t.Email == "" {
				use = t.Err
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", h.Host, t.Tool, acct, plan, use)
		}
	}
	_ = w.Flush()
	return b.String()
}

func usageLine(t ToolRow) string {
	if t.Usage.Err != "" && len(t.Usage.Windows) == 0 {
		return t.Usage.Err
	}
	var parts []string
	for _, w := range t.Usage.Windows {
		if w.Display != "" {
			parts = append(parts, w.Label+" "+w.Display)
		}
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, "  ")
}

func primaryPct(t ToolRow) string {
	for _, w := range t.Usage.Windows {
		if w.Utilization != nil {
			v := *w.Utilization
			if v > 0 && v <= 1 {
				v *= 100
			}
			return fmt.Sprintf("%.0f", v)
		}
	}
	return ""
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func compact(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		return s[:120]
	}
	return s
}

func WriteJSON(r Report, w *os.File) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// remoteIdentityPy prints only emails/plans. It never prints tokens.
const remoteIdentityPy = `
import json, os, pathlib, base64

def jwt_claims(tok):
    if not tok or tok.count(".") < 2:
        return {}
    p = tok.split(".")[1]
    p += "=" * ((4 - len(p) % 4) % 4)
    try:
        return json.loads(base64.urlsafe_b64decode(p.encode()))
    except Exception:
        return {}

home = pathlib.Path(os.path.expanduser("~"))
out = {"claude": {"tool": "claude"}, "codex": {"tool": "codex"}, "grok": {"tool": "grok"}}

p = home / ".claude.json"
if p.is_file():
    try:
        d = json.loads(p.read_text())
        oa = d.get("oauthAccount") or {}
        out["claude"]["email"] = oa.get("emailAddress") or ""
        plan = oa.get("organizationType") or ""
        tier = oa.get("organizationRateLimitTier") or ""
        out["claude"]["plan"] = (plan + " " + tier).strip()
        if not out["claude"]["email"]:
            out["claude"]["error"] = "not logged in"
    except Exception:
        out["claude"]["error"] = "claude.json: bad json"
else:
    out["claude"]["error"] = "no ~/.claude.json"

p = home / ".codex" / "auth.json"
if p.is_file():
    try:
        d = json.loads(p.read_text())
        tok = ((d.get("tokens") or {}).get("id_token")) or ""
        c = jwt_claims(tok)
        email = c.get("email") or ""
        prof = c.get("https://api.openai.com/profile") or {}
        if not email:
            email = prof.get("email") or ""
        auth = c.get("https://api.openai.com/auth") or {}
        out["codex"]["email"] = email
        out["codex"]["plan"] = auth.get("chatgpt_plan_type") or ""
        if not email:
            out["codex"]["error"] = "not logged in"
    except Exception:
        out["codex"]["error"] = "codex auth: bad json"
else:
    out["codex"]["error"] = "no ~/.codex/auth.json"

p = home / ".grok" / "auth.json"
if p.is_file():
    try:
        d = json.loads(p.read_text())
        found = False
        for v in d.values():
            if not isinstance(v, dict):
                continue
            email = v.get("email") or ""
            if not email:
                continue
            out["grok"]["email"] = email
            plan = v.get("auth_mode") or ""
            claims = jwt_claims(v.get("key") or "")
            if "tier" in claims:
                plan = (plan + " tier" + str(int(claims["tier"]))).strip()
            out["grok"]["plan"] = plan
            found = True
            break
        if not found:
            out["grok"]["error"] = "not logged in"
    except Exception:
        out["grok"]["error"] = "grok auth: bad json"
else:
    out["grok"]["error"] = "no ~/.grok/auth.json"

print(json.dumps(out))
`

// remoteUsagePy prints only usage windows. Tokens stay on the remote.
// Invoked as: ssh host python3 - <tool>
const remoteUsagePy = `
import json, os, pathlib, sys, urllib.request, urllib.error

tool = sys.argv[1] if len(sys.argv) > 1 else ""
home = pathlib.Path(os.path.expanduser("~"))

def pct(v):
    if v is None:
        return None
    v = float(v)
    if 0 < v <= 1:
        v *= 100
    return v

def window(label, util):
    if util is None:
        return None
    return {"label": label, "utilization": util, "display": f"{util:.0f}%"}

def get(url, headers):
    req = urllib.request.Request(url, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=8) as r:
            return r.status, json.loads(r.read().decode())
    except urllib.error.HTTPError as e:
        return e.code, None
    except Exception:
        return None, None

out = {"windows": [], "error": ""}

if tool == "claude":
    cred = home / ".claude" / ".credentials.json"
    tok = ""
    if cred.is_file():
        d = json.loads(cred.read_text())
        tok = ((d.get("claudeAiOauth") or {}).get("accessToken")) or ""
    if not tok:
        out["error"] = "no claude access token"
        print(json.dumps(out)); raise SystemExit
    status, body = get("https://api.anthropic.com/api/oauth/usage", {
        "Authorization": "Bearer " + tok,
        "Accept": "application/json",
        "User-Agent": "quota/0.1",
        "anthropic-beta": "oauth-2025-04-20",
    })
    if status != 200 or not body:
        out["error"] = f"HTTP {status or 'err'}"
        print(json.dumps(out)); raise SystemExit
    for key, label in (("five_hour", "5h"), ("seven_day", "wk")):
        w = body.get(key) or {}
        win = window(label, pct(w.get("utilization")))
        if win:
            out["windows"].append(win)
    if not out["windows"]:
        out["error"] = "claude usage: no windows"
elif tool == "codex":
    p = home / ".codex" / "auth.json"
    tok = ""
    if p.is_file():
        d = json.loads(p.read_text())
        tok = ((d.get("tokens") or {}).get("access_token")) or ""
    if not tok:
        out["error"] = "no access_token"
        print(json.dumps(out)); raise SystemExit
    status, body = get("https://chatgpt.com/backend-api/wham/usage", {
        "Authorization": "Bearer " + tok,
        "Accept": "application/json",
        "User-Agent": "quota/0.1",
    })
    if status != 200 or not body:
        out["error"] = f"HTTP {status or 'err'}"
        print(json.dumps(out)); raise SystemExit
    rl = body.get("rate_limit") or {}
    for key in ("primary_window", "secondary_window"):
        w = rl.get(key) or {}
        if not w:
            continue
        secs = w.get("limit_window_seconds") or 0
        label = "wk" if 600000 <= secs <= 620000 else ("5h" if 17000 <= secs <= 19000 else "win")
        util = w.get("used_percent")
        win = window(label, pct(util))
        if win:
            out["windows"].append(win)
    if not out["windows"]:
        out["error"] = "codex usage: no windows"
elif tool == "grok":
    p = home / ".grok" / "auth.json"
    tok = ""
    if p.is_file():
        d = json.loads(p.read_text())
        for v in d.values():
            if isinstance(v, dict) and v.get("key"):
                tok = v["key"]
                break
    if not tok:
        out["error"] = "no grok access token"
        print(json.dumps(out)); raise SystemExit
    status, body = get("https://cli-chat-proxy.grok.com/v1/billing?format=credits", {
        "Authorization": "Bearer " + tok,
        "Accept": "application/json",
        "User-Agent": "quota/0.1",
        "x-grok-client-mode": "cli",
    })
    if status != 200 or not body:
        out["error"] = f"HTTP {status or 'err'}"
        print(json.dumps(out)); raise SystemExit
    cfg = body.get("config") or body
    win = window("credits", pct(cfg.get("creditUsagePercent")))
    if win:
        out["windows"].append(win)
    else:
        out["error"] = "grok billing: no usage fields"
else:
    out["error"] = "unknown tool"

print(json.dumps(out))
`
