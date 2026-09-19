package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestResolvePrecedence(t *testing.T) {
	c := &Config{CurrentProfile: "one", Profiles: map[string]Profile{"one": {Token: "first"}, "two": {Token: "second"}}}
	t.Setenv("CLARITY_PROFILE", "two")
	t.Setenv("CLARITY_API_TOKEN", "environment")
	if token, _, _ := c.Resolve("one", "flag"); token != "flag" {
		t.Fatal(token)
	}
	if token, _, _ := c.Resolve("one", ""); token != "environment" {
		t.Fatal(token)
	}
	t.Setenv("CLARITY_API_TOKEN", "")
	if token, _, _ := c.Resolve("one", ""); token != "first" {
		t.Fatal(token)
	}
	if token, _, _ := c.Resolve("", ""); token != "second" {
		t.Fatal(token)
	}
	if _, _, err := c.Resolve("missing", ""); err == nil {
		t.Fatal("missing profile accepted")
	}
}

func TestAtomicConcurrentUpdatesAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := Update(context.Background(), path, func(c *Config) error { c.Profiles[fmt.Sprint(i)] = Profile{Token: "secret"}; return nil }); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Profiles) != 10 {
		t.Fatalf("lost updates: %d", len(c.Profiles))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("permissions %o", info.Mode().Perm())
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if err := Update(context.Background(), path, func(*Config) error { return nil }); err != nil {
			t.Fatal(err)
		}
		info, _ = os.Stat(path)
		if info.Mode().Perm() != 0600 {
			t.Fatal("failed to repair insecure existing permissions")
		}
	}
	if err := Update(context.Background(), path, func(c *Config) error { c.Profiles = map[string]Profile{}; return fmt.Errorf("abort") }); err == nil {
		t.Fatal("expected abort")
	}
	c, _ = Load(path)
	if len(c.Profiles) != 10 {
		t.Fatal("failed transaction modified config")
	}
}
