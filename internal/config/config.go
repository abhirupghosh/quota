package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Host struct {
	Name  string `json:"name"`
	SSH   string `json:"ssh,omitempty"` // empty means local
	Local bool   `json:"local,omitempty"`
}

type file struct {
	Hosts []Host `json:"hosts"`
}

func Home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func Dir() string {
	return filepath.Join(Home(), ".config", "quota")
}

func HostsPath() string {
	if p := os.Getenv("QUOTA_HOSTS"); p != "" {
		return p
	}
	return filepath.Join(Dir(), "hosts.json")
}

// Hosts is the machine list used by status and switch.
// Reads ~/.config/quota/hosts.json (or $QUOTA_HOSTS). If the file is missing
// or empty, the only host is this Mac.
func Hosts() []Host {
	list := loadHosts()
	out := make([]Host, 0, len(list)+1)
	hasLocal := false
	for _, h := range list {
		h.Name = strings.TrimSpace(h.Name)
		h.SSH = strings.TrimSpace(h.SSH)
		if h.Name == "" {
			continue
		}
		if h.SSH == "" {
			h.Local = true
		}
		if h.Local {
			hasLocal = true
		}
		out = append(out, h)
	}
	if !hasLocal {
		out = append([]Host{{Name: "mac", Local: true}}, out...)
	}
	return out
}

func Find(name string) (Host, error) {
	hosts := Hosts()
	names := make([]string, 0, len(hosts))
	for _, h := range hosts {
		names = append(names, h.Name)
		if h.Name == name || h.SSH == name {
			return h, nil
		}
	}
	return Host{}, fmt.Errorf("unknown host %q (have: %s)", name, strings.Join(names, "|"))
}

func InitHosts() (path string, created bool, err error) {
	path = HostsPath()
	if _, err := os.Stat(path); err == nil {
		return path, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return path, false, err
	}
	raw, err := json.MarshalIndent(file{Hosts: []Host{
		{Name: "mac", Local: true},
	}}, "", "  ")
	if err != nil {
		return path, false, err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return path, false, err
	}
	return path, true, nil
}

func loadHosts() []Host {
	raw, err := os.ReadFile(HostsPath())
	if err != nil {
		return nil
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil
	}
	return f.Hosts
}

func ClaudeJSON(home string) string {
	if home == "" {
		home = Home()
	}
	return filepath.Join(home, ".claude.json")
}

func ClaudeCreds(home string) string {
	if home == "" {
		home = Home()
	}
	return filepath.Join(home, ".claude", ".credentials.json")
}

func CodexAuth(home string) string {
	if home == "" {
		home = Home()
	}
	return filepath.Join(home, ".codex", "auth.json")
}

func GrokAuth(home string) string {
	if home == "" {
		home = Home()
	}
	return filepath.Join(home, ".grok", "auth.json")
}
