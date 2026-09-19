package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"go.yaml.in/yaml/v3"
)

type Profile struct {
	Token string `yaml:"token"`
}

type Config struct {
	CurrentProfile string             `yaml:"current_profile,omitempty"`
	Profiles       map[string]Profile `yaml:"profiles"`
}

func Path() (string, error) {
	if path := os.Getenv("CLARITY_CONFIG"); path != "" {
		return path, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "clarity-cli", "config.yaml"), nil
}

func Load(path string) (*Config, error) {
	c := &Config{Profiles: map[string]Profile{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	// Do not echo YAML parser errors: malformed lines can contain credentials.
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("invalid YAML config at %s", path)
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	return c, nil
}

func Update(ctx context.Context, path string, mutate func(*Config) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock := flock.New(path + ".lock")
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ok, err := lock.TryLockContext(lockCtx, 25*time.Millisecond)
	if err != nil {
		return fmt.Errorf("lock config: %w", err)
	}
	if !ok {
		return fmt.Errorf("config is locked by another process")
	}
	defer lock.Unlock()
	if err := os.Chmod(path+".lock", 0600); err != nil {
		return err
	}
	c, err := Load(path)
	if err != nil {
		return err
	}
	if err := mutate(c); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return atomicWrite(path, b)
}

func (c *Config) Resolve(profile, token string) (string, string, error) {
	if profile == "" {
		profile = os.Getenv("CLARITY_PROFILE")
	}
	if profile == "" {
		profile = c.CurrentProfile
	}
	if token != "" {
		return strings.TrimSpace(token), "flag", nil
	}
	if token = os.Getenv("CLARITY_API_TOKEN"); token != "" {
		return strings.TrimSpace(token), "environment", nil
	}
	if profile == "" {
		return "", "", fmt.Errorf("no token configured; run clarity auth login or set CLARITY_API_TOKEN")
	}
	p, ok := c.Profiles[profile]
	if !ok {
		return "", "", fmt.Errorf("profile %q not found", profile)
	}
	if strings.TrimSpace(p.Token) == "" {
		return "", "", fmt.Errorf("profile %q has no token; run clarity auth login --profile %s", profile, profile)
	}
	return strings.TrimSpace(p.Token), "profile:" + profile, nil
}
