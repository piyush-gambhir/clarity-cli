package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	// CacheFile holds the last release check, next to the config file.
	CacheFile = "update-check.json"
	// CheckInterval is how long a check (successful or failed) and a shown notice last.
	CheckInterval = 24 * time.Hour
)

// Cache records the last release check and the last update notice shown.
type Cache struct {
	CheckedAt       time.Time `json:"checked_at"`
	LatestVersion   string    `json:"latest_version,omitempty"`
	NotifiedVersion string    `json:"notified_version,omitempty"`
	NotifiedAt      time.Time `json:"notified_at,omitzero"`
}

// LoadCache reads the cache. A missing or corrupt file reads as empty, and a
// latest version that is not a release tag is dropped because it is printed.
func LoadCache(path string) Cache {
	var c Cache
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &c) != nil {
		return Cache{}
	}
	if !tagPattern.MatchString(c.LatestVersion) {
		c.LatestVersion = ""
	}
	return c
}

// Save writes the cache atomically with owner-only permissions.
func (c Cache) Save(path string) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".update-check-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Fresh reports whether the last check is recent enough to skip a lookup.
func (c Cache) Fresh(now time.Time) bool { return within(c.CheckedAt, now) }

// ShouldNotify reports whether to show a notice for the cached latest version:
// it is newer than current and was not already shown in the last day.
func (c Cache) ShouldNotify(current string, now time.Time) bool {
	if !Newer(c.LatestVersion, current) {
		return false
	}
	return c.NotifiedVersion != c.LatestVersion || !within(c.NotifiedAt, now)
}

func within(t, now time.Time) bool {
	age := now.Sub(t)
	return age >= 0 && age < CheckInterval
}
