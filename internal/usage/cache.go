package usage

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abhirupghosh/quota/internal/config"
	"github.com/abhirupghosh/quota/internal/identity"
)

const freshFor = 5 * time.Minute

type diskCache struct {
	Snapshot Snapshot  `json:"snapshot"`
	SavedAt  time.Time `json:"saved_at"`
	NextTry  time.Time `json:"next_try,omitempty"`
}

func cacheDir() string {
	return filepath.Join(config.Home(), ".config", "quota", "cache")
}

func cacheFile(tool identity.Tool, email string) string {
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(email))))
	name := string(tool) + "-" + hex.EncodeToString(sum[:8]) + ".json"
	return filepath.Join(cacheDir(), name)
}

// CachedFetch skips the network when a snapshot is younger than 5 minutes.
// On 429/401/errors it returns the last good snapshot so the menu does not go blank.
func CachedFetch(tool identity.Tool, home, email string) Snapshot {
	if email != "" {
		if s, ok := recall(tool, email, true); ok {
			return s
		}
	}
	s := Fetch(tool, home)
	s.Account = email
	if s.Err == "" && len(s.Windows) > 0 {
		remember(s)
		return s
	}
	if email != "" {
		if stale, ok := recall(tool, email, false); ok {
			stale.Stale = true
			if s.Err != "" {
				stale.Err = s.Err
			}
			remember(stale)
			return stale
		}
	}
	return s
}

func Remember(s Snapshot) {
	if s.Account == "" || len(s.Windows) == 0 {
		return
	}
	remember(s)
}

func Recall(tool identity.Tool, email string) (Snapshot, bool) {
	return recall(tool, email, false)
}

func remember(s Snapshot) {
	if s.Tool == "" || s.Account == "" {
		return
	}
	_ = os.MkdirAll(cacheDir(), 0o700)
	doc := diskCache{Snapshot: Snapshot{Tool: s.Tool, Account: s.Account, Windows: s.Windows}, SavedAt: time.Now()}
	if strings.Contains(s.Err, "429") {
		doc.NextTry = time.Now().Add(15 * time.Minute)
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return
	}
	path := cacheFile(s.Tool, s.Account)
	_ = os.WriteFile(path, append(b, '\n'), 0o600)
}

func recall(tool identity.Tool, email string, freshOnly bool) (Snapshot, bool) {
	raw, err := os.ReadFile(cacheFile(tool, email))
	if err != nil {
		return Snapshot{}, false
	}
	var doc diskCache
	if json.Unmarshal(raw, &doc) != nil || len(doc.Snapshot.Windows) == 0 {
		return Snapshot{}, false
	}
	age := time.Since(doc.SavedAt)
	if freshOnly {
		if !doc.NextTry.IsZero() && time.Now().Before(doc.NextTry) {
			s := doc.Snapshot
			s.Stale = age > freshFor
			return s, true
		}
		if age > freshFor {
			return Snapshot{}, false
		}
	}
	s := doc.Snapshot
	if age > freshFor {
		s.Stale = true
	}
	return s, true
}
