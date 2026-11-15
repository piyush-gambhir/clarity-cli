package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	CacheFile = "update-check.json"
	CheckInterval = 24 * time.Hour
)

type Cache struct {
	CheckedAt       time.Time `json:"checked_at"`
	LatestVersion   string    `json:"latest_version,omitempty"`
	NotifiedVersion string    `json:"notified_version,omitempty"`
	NotifiedAt      time.Time `json:"notified_at,omitzero"`
}

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

func (c Cache) Fresh(now time.Time) bool { return within(c.CheckedAt, now) }

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
