package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHostsMissingFileIsLocalOnly(t *testing.T) {
	t.Setenv("QUOTA_HOSTS", filepath.Join(t.TempDir(), "nope.json"))
	got := Hosts()
	if len(got) != 1 || !got[0].Local || got[0].Name != "mac" {
		t.Fatalf("got %#v", got)
	}
}

func TestHostsReadsFileAndPrependsLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.json")
	if err := os.WriteFile(path, []byte(`{"hosts":[{"name":"devbox","ssh":"devbox"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUOTA_HOSTS", path)
	got := Hosts()
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
	if !got[0].Local || got[0].Name != "mac" {
		t.Fatalf("local %#v", got[0])
	}
	if got[1].Name != "devbox" || got[1].SSH != "devbox" || got[1].Local {
		t.Fatalf("remote %#v", got[1])
	}
}

func TestFind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.json")
	raw := `{"hosts":[{"name":"mac","local":true},{"name":"ubuntu","ssh":"ubuntu-box"}]}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUOTA_HOSTS", path)
	h, err := Find("ubuntu-box")
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "ubuntu" {
		t.Fatalf("name %q", h.Name)
	}
	_, err = Find("nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInitHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.json")
	t.Setenv("QUOTA_HOSTS", path)
	got, created, err := InitHosts()
	if err != nil || !created || got != path {
		t.Fatalf("created=%v path=%s err=%v", created, got, err)
	}
	got, created, err = InitHosts()
	if err != nil || created {
		t.Fatalf("second created=%v err=%v", created, err)
	}
}
