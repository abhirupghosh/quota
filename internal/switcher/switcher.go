package switcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/abhirupghosh/quota/internal/config"
)

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,31}$`)

type Meta struct {
	Name   string `json:"name"`
	Tool   string `json:"tool"`
	Email  string `json:"email"`
	Plan   string `json:"plan,omitempty"`
	Source string `json:"source,omitempty"`
	Saved  string `json:"saved_at"`
}

func StoreDir(home string) string {
	if home == "" {
		home = config.Home()
	}
	return filepath.Join(home, ".config", "quota", "accounts")
}

func ClaudeDir(home, name string) string {
	return filepath.Join(StoreDir(home), "claude", name)
}

func FindHost(name string) (config.Host, error) {
	return config.Find(name)
}

func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("bad snapshot name %q", name)
	}
	return nil
}

func List(home string) ([]Meta, error) {
	dir := filepath.Join(StoreDir(home), "claude")
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Meta
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		m, err := readMeta(filepath.Join(dir, e.Name(), "meta.json"))
		if err != nil {
			out = append(out, Meta{Name: e.Name(), Tool: "claude", Email: "?"})
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func SaveLocal(home, name, source string) (Meta, error) {
	if err := ValidateName(name); err != nil {
		return Meta{}, err
	}
	if home == "" {
		home = config.Home()
	}
	oauth, err := readOAuthAccount(config.ClaudeJSON(home))
	if err != nil {
		return Meta{}, err
	}
	email, _ := oauth["emailAddress"].(string)
	if email == "" {
		return Meta{}, fmt.Errorf("claude is not logged in")
	}
	creds, err := readClaudeOauthBlob(home)
	if err != nil {
		return Meta{}, err
	}
	if err := usableBlob(creds); err != nil {
		return Meta{}, err
	}
	dir := ClaudeDir(home, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Meta{}, err
	}
	if err := writeJSON(filepath.Join(dir, "oauth.json"), oauth); err != nil {
		return Meta{}, err
	}
	if err := writeJSON(filepath.Join(dir, "credentials.json"), map[string]any{"claudeAiOauth": creds}); err != nil {
		return Meta{}, err
	}
	plan, _ := oauth["organizationType"].(string)
	if t, _ := oauth["organizationRateLimitTier"].(string); t != "" {
		if plan != "" {
			plan += " "
		}
		plan += t
	}
	meta := Meta{Name: name, Tool: "claude", Email: email, Plan: plan, Source: source, Saved: time.Now().UTC().Format(time.RFC3339)}
	if err := writeJSON(filepath.Join(dir, "meta.json"), meta); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

func UseLocal(home, name string) (Meta, error) {
	if err := ValidateName(name); err != nil {
		return Meta{}, err
	}
	if home == "" {
		home = config.Home()
	}
	dir := ClaudeDir(home, name)
	meta, err := readMeta(filepath.Join(dir, "meta.json"))
	if err != nil {
		return Meta{}, fmt.Errorf("snapshot %q: %w", name, err)
	}
	oauth, err := readMap(filepath.Join(dir, "oauth.json"))
	if err != nil {
		return Meta{}, err
	}
	credsFile, err := readMap(filepath.Join(dir, "credentials.json"))
	if err != nil {
		return Meta{}, err
	}
	blob, ok := credsFile["claudeAiOauth"].(map[string]any)
	if !ok {
		return Meta{}, fmt.Errorf("snapshot %q missing claudeAiOauth", name)
	}
	if err := usableBlob(blob); err != nil {
		return Meta{}, fmt.Errorf("snapshot %q: %w", name, err)
	}
	if err := mergeOAuthAccount(config.ClaudeJSON(home), oauth); err != nil {
		return Meta{}, err
	}
	if err := mergeClaudeOauthBlob(home, blob); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

func SaveRemote(h config.Host, name string) (Meta, error) {
	if h.Local || h.SSH == "" {
		return SaveLocal("", name, "mac")
	}
	if err := ValidateName(name); err != nil {
		return Meta{}, err
	}
	script := fmt.Sprintf(remoteSavePy, name)
	if err := sshRun(h.SSH, script); err != nil {
		return Meta{}, err
	}
	local := ClaudeDir("", name)
	if err := os.MkdirAll(filepath.Dir(local), 0o700); err != nil {
		return Meta{}, err
	}
	_ = os.RemoveAll(local)
	if err := scpFrom(h.SSH, ".config/quota/accounts/claude/"+name, local); err != nil {
		return Meta{}, err
	}
	return readMeta(filepath.Join(local, "meta.json"))
}

func UseRemote(h config.Host, name string) (Meta, error) {
	if h.Local || h.SSH == "" {
		return UseLocal("", name)
	}
	if err := ValidateName(name); err != nil {
		return Meta{}, err
	}
	local := ClaudeDir("", name)
	if _, err := os.Stat(filepath.Join(local, "meta.json")); err != nil {
		return Meta{}, fmt.Errorf("snapshot %q not on this Mac — save it first", name)
	}
	if err := sshShell(h.SSH, "mkdir -p ~/.config/quota/accounts/claude && chmod 700 ~/.config/quota ~/.config/quota/accounts ~/.config/quota/accounts/claude && rm -rf ~/.config/quota/accounts/claude/"+name); err != nil {
		return Meta{}, err
	}
	if err := scpTo(h.SSH, local, ".config/quota/accounts/claude/"+name); err != nil {
		return Meta{}, err
	}
	if err := sshRun(h.SSH, fmt.Sprintf(remoteUsePy, name)); err != nil {
		return Meta{}, err
	}
	return readMeta(filepath.Join(local, "meta.json"))
}

func PushHost(h config.Host) error {
	if h.Local || h.SSH == "" {
		return nil
	}
	src := filepath.Join(StoreDir(""), "claude")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("nothing to push")
	}
	if err := sshShell(h.SSH, "mkdir -p ~/.config/quota/accounts && chmod 700 ~/.config/quota ~/.config/quota/accounts && rm -rf ~/.config/quota/accounts/claude"); err != nil {
		return err
	}
	return scpTo(h.SSH, src, ".config/quota/accounts/claude")
}

func usableBlob(blob map[string]any) error {
	if blob == nil {
		return fmt.Errorf("empty credential blob")
	}
	at, _ := blob["accessToken"].(string)
	rt, _ := blob["refreshToken"].(string)
	if len(rt) < 20 {
		return fmt.Errorf("refresh token missing or stub (%d chars); Claude would ask you to log in. Save from a host that is currently logged in", len(rt))
	}
	if len(at) < 20 && len(rt) < 20 {
		return fmt.Errorf("access token stub (%d chars)", len(at))
	}
	return nil
}

func readClaudeOauthBlob(home string) (map[string]any, error) {
	path := config.ClaudeCreds(home)
	if raw, err := os.ReadFile(path); err == nil {
		var root map[string]any
		if json.Unmarshal(raw, &root) == nil {
			if blob, ok := root["claudeAiOauth"].(map[string]any); ok && blob["accessToken"] != nil {
				return blob, nil
			}
		}
	}
	if out, err := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output(); err == nil {
		var root map[string]any
		if json.Unmarshal(bytes.TrimSpace(out), &root) == nil {
			if blob, ok := root["claudeAiOauth"].(map[string]any); ok && blob["accessToken"] != nil {
				return blob, nil
			}
			if root["accessToken"] != nil {
				return root, nil
			}
		}
	}
	return nil, fmt.Errorf("no Claude access token on this host")
}

func mergeClaudeOauthBlob(home string, blob map[string]any) error {
	path := config.ClaudeCreds(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	root := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &root)
	}
	root["claudeAiOauth"] = blob
	if err := writeJSON(path, root); err != nil {
		return err
	}
	// macOS Keychain is what Claude Code actually reads on this Mac.
	raw, err := json.Marshal(root)
	if err != nil {
		return err
	}
	_ = exec.Command("security", "add-generic-password", "-U", "-s", "Claude Code-credentials", "-a", os.Getenv("USER"), "-w", string(raw)).Run()
	return nil
}

func readOAuthAccount(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no ~/.claude.json")
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("claude.json: bad json")
	}
	oa, _ := root["oauthAccount"].(map[string]any)
	if oa == nil {
		return nil, fmt.Errorf("claude is not logged in")
	}
	return oa, nil
}

func mergeOAuthAccount(path string, oa map[string]any) error {
	root := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &root)
	}
	root["oauthAccount"] = oa
	return writeJSON(path, root)
}

func readMeta(path string) (Meta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(raw, &m); err != nil {
		return Meta{}, err
	}
	return m, nil
}

func readMap(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func sshShell(host, cmd string) error {
	c := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=12", host, cmd)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ssh %s: %s", host, compact(msg))
	}
	return nil
}

func sshRun(host, script string) error {
	cmd := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=12", host, "python3", "-")
	cmd.Stdin = strings.NewReader(script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = os.Stderr // only our scripts print emails/status, never tokens
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ssh %s: %s", host, compact(msg))
	}
	return nil
}

func scpTo(host, local, remote string) error {
	cmd := exec.Command("scp", "-q", "-o", "BatchMode=yes", "-o", "ConnectTimeout=12", "-r", local, host+":"+remote)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp to %s: %s", host, compact(stderr.String()))
	}
	return nil
}

func scpFrom(host, remote, local string) error {
	cmd := exec.Command("scp", "-q", "-o", "BatchMode=yes", "-o", "ConnectTimeout=12", "-r", host+":"+remote, local)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp from %s: %s", host, compact(stderr.String()))
	}
	return nil
}

func compact(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

const remoteSavePy = `
import json, os, pathlib, time
name = %q
home = pathlib.Path.home()
oa_path = home / ".claude.json"
cred_path = home / ".claude" / ".credentials.json"
d = json.loads(oa_path.read_text())
oa = d.get("oauthAccount") or {}
email = oa.get("emailAddress") or ""
if not email:
    raise SystemExit("claude is not logged in")
creds = json.loads(cred_path.read_text())
blob = creds.get("claudeAiOauth")
if not blob:
    raise SystemExit("no claude access token")
rt = blob.get("refreshToken") or ""
at = blob.get("accessToken") or ""
if len(rt) < 20:
    raise SystemExit("refresh token missing or stub; login on this host first")
dest = home / ".config" / "quota" / "accounts" / "claude" / name
dest.mkdir(parents=True, exist_ok=True)
os.chmod(dest.parent.parent.parent, 0o700)
os.chmod(dest.parent.parent, 0o700)
os.chmod(dest.parent, 0o700)
os.chmod(dest, 0o700)
(dest / "oauth.json").write_text(json.dumps(oa, indent=2) + "\n")
(dest / "credentials.json").write_text(json.dumps({"claudeAiOauth": blob}, indent=2) + "\n")
plan = " ".join(x for x in [oa.get("organizationType") or "", oa.get("organizationRateLimitTier") or ""] if x)
meta = {"name": name, "tool": "claude", "email": email, "plan": plan, "source": "remote", "saved_at": time.strftime("%%Y-%%m-%%dT%%H:%%M:%%SZ", time.gmtime())}
(dest / "meta.json").write_text(json.dumps(meta, indent=2) + "\n")
for p in dest.iterdir():
    os.chmod(p, 0o600)
print("saved", name, email)
`

const remoteUsePy = `
import json, os, pathlib
name = %q
home = pathlib.Path.home()
src = home / ".config" / "quota" / "accounts" / "claude" / name
oa = json.loads((src / "oauth.json").read_text())
blob = json.loads((src / "credentials.json").read_text()).get("claudeAiOauth")
if not blob:
    raise SystemExit("snapshot missing claudeAiOauth")
if len(blob.get("refreshToken") or "") < 20:
    raise SystemExit("snapshot refresh token is a stub; re-save from a logged-in host")
p = home / ".claude.json"
root = json.loads(p.read_text()) if p.is_file() else {}
root["oauthAccount"] = oa
p.write_text(json.dumps(root, indent=2) + "\n")
os.chmod(p, 0o600)
cdir = home / ".claude"
cdir.mkdir(exist_ok=True)
os.chmod(cdir, 0o700)
cp = cdir / ".credentials.json"
cur = json.loads(cp.read_text()) if cp.is_file() else {}
cur["claudeAiOauth"] = blob
cp.write_text(json.dumps(cur, indent=2) + "\n")
os.chmod(cp, 0o600)
print("using", name, oa.get("emailAddress"))
`
