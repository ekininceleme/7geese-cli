// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	BaseURL        string `json:"base_url"`
	AuthHeaderVal  string `json:"auth_header"`
	Headers        map[string]string `json:"headers,omitempty"`
	AuthSource     string `json:"-"`
	AccessToken    string `json:"access_token"`
	RefreshToken   string `json:"refresh_token"`
	TokenExpiry    time.Time `json:"token_expiry"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	Path           string `json:"-"`
	SevengeeseSession string `json:"session"`
	SevengeeseCSRF    string `json:"csrf,omitempty"`
}

func Load(configPath string) (*Config, error) {
	cfg := &Config{
		BaseURL: "https://app.7geese.com",
	}

	// Resolve config path
	path := configPath
	if path == "" {
		path = os.Getenv("API_7GEESE_CONFIG")
	}
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".config", "7geese-cli", "config.json")
	}
	cfg.Path = path

	// Try to load config file
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config %s: %w", path, err)
		}
		cfg.Path = path
	}

	// Env var overrides
	if v := os.Getenv("SEVENGEESE_SESSION"); v != "" {
		cfg.SevengeeseSession = v
		cfg.AuthSource = "env:SEVENGEESE_SESSION"
	}
	if v := os.Getenv("SEVENGEESE_CSRF"); v != "" {
		cfg.SevengeeseCSRF = v
	}

	// Rebuild Cookie header from stored session + CSRF so the client sends them
	if cfg.SevengeeseSession != "" {
		if cfg.Headers == nil {
			cfg.Headers = map[string]string{}
		}
		cookie := "sgsession4=" + cfg.SevengeeseSession
		if cfg.SevengeeseCSRF != "" {
			cookie += "; sgcsrftoken4=" + cfg.SevengeeseCSRF
		}
		cfg.Headers["Cookie"] = cookie
		if cfg.SevengeeseCSRF != "" {
			cfg.Headers["X-CSRFToken"] = cfg.SevengeeseCSRF
		}
	}

	// Label config-file-derived credentials so doctor can distinguish
	// "credentials persisted on disk" from "no credentials at all" — without
	// this, users who saved via set-token without an env var see a blank
	// auth_source and can't tell whether their config is being picked up.
	// The label is the literal "config" rather than "config:<path>"; the
	// config file path is exposed separately as report["config_path"], and
	// embedding it in auth_source leaks the user's home directory through
	// doctor's JSON envelope.
	if cfg.AuthSource == "" && (cfg.AuthHeaderVal != "" || cfg.AccessToken != "") {
		cfg.AuthSource = "config"
	}
	if cfg.AuthSource == "" && cfg.SevengeeseSession != "" {
		cfg.AuthSource = "config"
	}

	// Base URL override (env var for testing).
	if v := os.Getenv("API_7GEESE_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	return cfg, nil
}

func (c *Config) AuthHeader() string {
	if c.AuthHeaderVal != "" {
		return c.AuthHeaderVal
	}
	// Env-var token wins over file-stored AccessToken (env > config convention).
	if c.SevengeeseSession != "" {
		c.AuthSource = "env:SEVENGEESE_SESSION"
		return c.SevengeeseSession
	}
	if c.AccessToken != "" {
		c.AuthSource = "browser"
		return c.AccessToken
	}
	return ""
}

func applyAuthFormat(format string, replacements map[string]string) string {
	if format == "" {
		return ""
	}
	for key, value := range replacements {
		format = strings.ReplaceAll(format, "{"+key+"}", value)
	}
	if strings.Contains(format, "{") {
		return ""
	}
	return format
}

func (c *Config) SaveTokens(clientID, clientSecret, accessToken, refreshToken string, expiry time.Time) error {
	c.ClientID = clientID
	c.ClientSecret = clientSecret
	c.AccessToken = accessToken
	c.RefreshToken = refreshToken
	c.TokenExpiry = expiry
	return c.save()
}

// SaveCookies stores the 7Geese session + CSRF cookies and updates the request headers.
func (c *Config) SaveCookies(session, csrf string) error {
	c.SevengeeseSession = session
	c.SevengeeseCSRF = csrf
	if c.Headers == nil {
		c.Headers = map[string]string{}
	}
	cookie := "sgsession4=" + session
	if csrf != "" {
		cookie += "; sgcsrftoken4=" + csrf
	}
	c.Headers["Cookie"] = cookie
	if csrf != "" {
		c.Headers["X-CSRFToken"] = csrf
	}
	c.AuthSource = "config"
	return c.save()
}

func (c *Config) ClearTokens() error {
	c.AccessToken = ""
	c.RefreshToken = ""
	c.TokenExpiry = time.Time{}
	c.SevengeeseSession = ""
	c.SevengeeseCSRF = ""
	if c.Headers != nil {
		delete(c.Headers, "Cookie")
		delete(c.Headers, "X-CSRFToken")
	}
	return c.save()
}

func (c *Config) save() error {
	dir := filepath.Dir(c.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(c.Path, data, 0o600)
}

// Ensure strings import is used
var _ = strings.ReplaceAll
