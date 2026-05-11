// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"7geese-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/steipete/sweetcookie"
)

const sevengeeseURL = "https://app.7geese.com/"
const sessionCookieName = "sgsession4"
const csrfCookieName = "sgcsrftoken4"

func newAuthCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication for 7Geese",
	}
	cmd.AddCommand(newAuthLoginCmd(flags))
	cmd.AddCommand(newAuthStatusCmd(flags))
	cmd.AddCommand(newAuthLogoutCmd(flags))
	return cmd
}

func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var useChrome, useFirefox, useSafari bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Read your 7Geese session from Chrome, Firefox, or Safari",
		Long: `Reads your existing 7Geese browser session using sweetcookie.

No API key is needed — just log into app.7geese.com via Okta in your browser.

  --chrome   Read from Google Chrome (default)
  --firefox  Read from Mozilla Firefox
  --safari   Read from Safari (macOS only)

With no flag, Chrome is tried first, then Firefox.`,
		Example: strings.Trim(`
  7geese-cli auth login
  7geese-cli auth login --chrome
  7geese-cli auth login --firefox`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}

			w := cmd.OutOrStdout()

			browsers := resolveBrowsers(useChrome, useFirefox, useSafari)

			fmt.Fprintf(w, "Reading 7Geese session from %s...\n", browserNames(browsers))

			session, csrf, browser, warnings, err := extractSweetCookies(browsers)
			if err != nil {
				fmt.Fprintln(w, red("Could not read session cookies."))
				for _, warn := range warnings {
					if strings.Contains(warn, "keychain") {
						fmt.Fprintln(w, "")
						fmt.Fprintf(w, "  Keychain error: %s\n", warn)
						fmt.Fprintln(w, "  If macOS prompted for keychain access, click Allow and try again.")
						fmt.Fprintln(w, "  Or open Keychain Access → look for 'Chrome Safe Storage' → grant access.")
					}
				}
				fmt.Fprintln(w, "")
				fmt.Fprintln(w, "Make sure you are logged into app.7geese.com in your browser, then try again:")
				fmt.Fprintf(w, "  7geese-cli auth login --chrome\n")
				fmt.Fprintf(w, "  7geese-cli auth login --firefox\n")
				return authErr(err)
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			if err := cfg.SaveCookies(session, csrf); err != nil {
				return configErr(fmt.Errorf("saving cookies: %w", err))
			}

			fmt.Fprintf(w, "%s Authenticated via %s session\n", green("OK"), browser)
			fmt.Fprintf(w, "  Config: %s\n", cfg.Path)
			fmt.Fprintf(w, "  Session expires when you log out of 7Geese in your browser.\n")
			fmt.Fprintf(w, "  Re-run this command if you get 401 errors.\n")
			return nil
		},
	}

	cmd.Flags().BoolVar(&useChrome, "chrome", false, "Read cookies from Chrome")
	cmd.Flags().BoolVar(&useFirefox, "firefox", false, "Read cookies from Firefox")
	cmd.Flags().BoolVar(&useSafari, "safari", false, "Read cookies from Safari (macOS only)")
	cmd.Flags().BoolVar(&useChrome, "browser", false, "Alias for --chrome")
	return cmd
}

// extractSweetCookies tries each browser in order and returns (session, csrf, browserName, warnings, error).
func extractSweetCookies(browsers []sweetcookie.Browser) (string, string, string, []string, error) {
	res, err := sweetcookie.Get(context.Background(), sweetcookie.Options{
		URL:      sevengeeseURL,
		Names:    []string{sessionCookieName, csrfCookieName},
		Browsers: browsers,
		Mode:     sweetcookie.ModeFirst,
	})
	if err != nil {
		return "", "", "", nil, fmt.Errorf("sweetcookie: %w", err)
	}

	var session, csrf, browser string
	for _, c := range res.Cookies {
		switch c.Name {
		case sessionCookieName:
			session = c.Value
			browser = string(c.Source.Browser)
		case csrfCookieName:
			csrf = c.Value
		}
	}

	if session == "" {
		return "", "", "", res.Warnings, fmt.Errorf("no %s cookie found for app.7geese.com — log in via Okta first", sessionCookieName)
	}
	return session, csrf, browser, res.Warnings, nil
}

// resolveBrowsers returns the ordered list of browsers to try.
func resolveBrowsers(chrome, firefox, safari bool) []sweetcookie.Browser {
	if !chrome && !firefox && !safari {
		// Default: Chrome first, Firefox fallback
		return []sweetcookie.Browser{sweetcookie.BrowserChrome, sweetcookie.BrowserFirefox}
	}
	var out []sweetcookie.Browser
	if chrome {
		out = append(out, sweetcookie.BrowserChrome)
	}
	if firefox {
		out = append(out, sweetcookie.BrowserFirefox)
	}
	if safari {
		out = append(out, sweetcookie.BrowserSafari)
	}
	return out
}

func browserNames(browsers []sweetcookie.Browser) string {
	names := make([]string, len(browsers))
	for i, b := range browsers {
		names[i] = string(b)
	}
	return strings.Join(names, " then ")
}

func newAuthStatusCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Short:   "Show authentication status",
		Example: "  7geese-cli auth status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}

			w := cmd.OutOrStdout()

			if cfg.SevengeeseSession != "" {
				fmt.Fprintln(w, green("Authenticated"))
				fmt.Fprintf(w, "  Source:  %s\n", cfg.AuthSource)
				fmt.Fprintf(w, "  Session: %s...\n", cfg.SevengeeseSession[:min(8, len(cfg.SevengeeseSession))])
				fmt.Fprintf(w, "  CSRF:    %v\n", cfg.SevengeeseCSRF != "")
				fmt.Fprintf(w, "  Config:  %s\n", cfg.Path)
				return nil
			}

			if v := os.Getenv("SEVENGEESE_SESSION"); v != "" {
				fmt.Fprintln(w, green("Authenticated"))
				fmt.Fprintf(w, "  Source: env:SEVENGEESE_SESSION\n")
				return nil
			}

			fmt.Fprintln(w, red("Not authenticated"))
			fmt.Fprintln(w, "")
			fmt.Fprintln(w, "Log in from your browser session:")
			fmt.Fprintf(w, "  7geese-cli auth login --chrome\n")
			return authErr(fmt.Errorf("no credentials configured"))
		},
	}
}

func newAuthLogoutCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "logout",
		Short:   "Clear stored session cookies",
		Example: "  7geese-cli auth logout",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			if err := cfg.ClearTokens(); err != nil {
				return configErr(fmt.Errorf("clearing tokens: %w", err))
			}
			if os.Getenv("SEVENGEESE_SESSION") != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Config cleared. Note: SEVENGEESE_SESSION env var is still set.\n")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Logged out. Session cookies cleared.")
			return nil
		},
	}
}

// TryRefreshSession attempts to re-read the 7Geese session from the browser.
// Called automatically by the client on 401. Returns true if a new session was saved.
func TryRefreshSession(configPath string) bool {
	session, csrf, _, _, err := extractSweetCookies(
		[]sweetcookie.Browser{sweetcookie.BrowserChrome, sweetcookie.BrowserFirefox},
	)
	if err != nil || session == "" {
		return false
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return false
	}
	return cfg.SaveCookies(session, csrf) == nil
}

// parseCookieString splits "name=val; name2=val2" into a map (kept for compatibility).
func parseCookieString(cookies string) map[string]string {
	m := make(map[string]string)
	for _, pair := range strings.Split(cookies, "; ") {
		pair = strings.TrimSpace(pair)
		if idx := strings.IndexByte(pair, '='); idx >= 0 {
			m[pair[:idx]] = pair[idx+1:]
		}
	}
	return m
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Stub kept so existing generated files that call SaveTokens still compile.
var _ = time.Time{}
var _ = json.Marshal
