package usage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abhirupghosh/quota/internal/identity"
)

func TestRememberRecall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	u := 22.0
	s := Snapshot{
		Tool:    identity.Claude,
		Account: "a@x.ai",
		Windows: []Window{{Label: "5h", Utilization: &u, Display: "22%"}},
	}
	remember(s)
	got, ok := recall(identity.Claude, "a@x.ai", true)
	if !ok || len(got.Windows) != 1 {
		t.Fatalf("recall: %+v ok=%v", got, ok)
	}
	if got.Windows[0].Display != "22%" {
		t.Fatalf("display %q", got.Windows[0].Display)
	}
	path := cacheFile(identity.Claude, "a@x.ai")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != cacheDir() {
		t.Fatal(path)
	}
}
