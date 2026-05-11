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
	"github.com/browserutils/kooky"
	_ "github.com/browserutils/kooky/browser/chrome"
	_ "github.com/browserutils/kooky/browser/firefox"
	_ "github.com/browserutils/kooky/browser/safari"
	"github.com/spf13/cobra"
)

const sevengeeseURL = "https://app.7geese.com/"
const sessionCookieName = "sgsession4"
const csrfCookieName = "sgcsrftoken4"

const browserChrome = "chrome"
const browserFirefox = "firefox"
const browserSafari = "safari"

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
		Long: `Reads your existing 7Geese browser session using kooky.

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

			browsers := resolveBrowserNames(useChrome, useFirefox, useSafari)

			fmt.Fprintf(w, "Reading 7Geese session from %s...\n", strings.Join(browsers, " then "))

			session, csrf, browser, err := extractKookyCookies(browsers)
			if err != nil {
				fmt.Fprintln(w, red("Could not read session cookies."))
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

// extractKookyCookies reads the 7Geese session and CSRF cookies directly from
// the browser's cookie store using kooky (CGO + Security.framework on macOS,
// no subprocess). Returns (session, csrf, browserName, error).
func extractKookyCookies(browsers []string) (string, string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	browserSet := make(map[string]bool, len(browsers))
	for _, b := range browsers {
		browserSet[strings.ToLower(b)] = true
	}

	filters := []kooky.Filter{
		kooky.Valid,
		kooky.DomainHasSuffix("7geese.com"),
		kooky.FilterFunc(func(c *kooky.Cookie) bool {
			if len(browsers) == 0 || c.Browser == nil {
				return true
			}
			return browserSet[strings.ToLower(c.Browser.Browser())]
		}),
	}

	var session, csrf, browserName string
	for cookie := range kooky.TraverseCookies(ctx, filters...).OnlyCookies() {
		switch cookie.Name {
		case sessionCookieName:
			if session == "" {
				session = cookie.Value
				if cookie.Browser != nil {
					browserName = cookie.Browser.Browser()
				}
			}
		case csrfCookieName:
			if csrf == "" {
				csrf = cookie.Value
			}
		}
		if session != "" && csrf != "" {
			break
		}
	}

	if session == "" {
		return "", "", "", fmt.Errorf("no %s cookie found for app.7geese.com — log in via Okta first", sessionCookieName)
	}
	return session, csrf, browserName, nil
}

// resolveBrowserNames returns the ordered list of browser name strings to try.
func resolveBrowserNames(chrome, firefox, safari bool) []string {
	if !chrome && !firefox && !safari {
		return []string{browserChrome, browserFirefox}
	}
	var out []string
	if chrome {
		out = append(out, browserChrome)
	}
	if firefox {
		out = append(out, browserFirefox)
	}
	if safari {
		out = append(out, browserSafari)
	}
	return out
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
	session, csrf, _, err := extractKookyCookies([]string{browserChrome, browserFirefox})
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
