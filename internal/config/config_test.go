// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuthHeader_KeepsAuthSource(t *testing.T) {
	t.Setenv("SEVENGEESE_SESSION", "")
	t.Setenv("SEVENGEESE_CSRF", "")
	t.Setenv("API_7GEESE_BASE_URL", "")
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"session":"file-session"}`), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.AuthSource != "config" {
		t.Fatalf("AuthSource = %q, want config", cfg.AuthSource)
	}
	if header := cfg.AuthHeader(); header != "file-session" {
		t.Errorf("AuthHeader = %q, want file-session", header)
	}
	if cfg.AuthSource != "config" {
		t.Errorf("AuthSource after AuthHeader = %q, want config", cfg.AuthSource)
	}

	t.Setenv("SEVENGEESE_SESSION", "env-session")
	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if header := cfg.AuthHeader(); header != "env-session" {
		t.Errorf("AuthHeader = %q, want env-session", header)
	}
	if cfg.AuthSource != "env:SEVENGEESE_SESSION" {
		t.Errorf("AuthSource = %q, want env:SEVENGEESE_SESSION", cfg.AuthSource)
	}
}
