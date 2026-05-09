# 7geese-cli

Export your 7Geese performance data — objectives, 1:1s, recognitions, and reviews — as JSON.

7Geese has no API tokens and no official CLI. This tool reads your existing browser session, syncs your data to a local SQLite store, and exports everything to a single JSON file.

## Install

```bash
brew tap ekininceleme/tap
brew install sevengeese-cli
```

Or download a pre-built binary for macOS, Linux, or Windows from the [latest release](https://github.com/ekininceleme/7geese-cli/releases/latest). On macOS, clear the quarantine flag after downloading:

```bash
xattr -d com.apple.quarantine 7geese-cli && chmod +x 7geese-cli
```

## Quick start

```bash
# Just run export — it will prompt to auth and sync if needed
7geese-cli me export --output my-data.json
```

Or step by step:

```bash
7geese-cli auth login   # read session from Chrome or Firefox
7geese-cli sync         # pull data into a local database
7geese-cli me export --output my-data.json
```

## Commands

### `auth login`

Reads your `sgsession4` session cookie from your browser's encrypted cookie store — no API key or password needed. Works with Chrome and Firefox. You must already be logged into 7Geese in your browser.

```bash
7geese-cli auth login           # tries Chrome first, then Firefox
7geese-cli auth login --chrome
7geese-cli auth login --firefox
```

**macOS**: the OS will show a dialog asking permission to access the login keychain. Click **Allow** (or **Always Allow** to avoid the prompt in future).

### `sync`

Pulls your objectives, 1:1s, recognitions, and reviews into a local SQLite database at `~/.local/share/7geese-cli/data.db`. Re-run any time to pick up new data.

```bash
7geese-cli sync
```

### `me export`

Exports all your performance data to a single JSON file.

```bash
# Write to a file
7geese-cli me export --output my-data.json

# Print to stdout (pipe to redirect)
7geese-cli me export > my-data.json
7geese-cli me export | jq '.objectives'

# Only include data from 2025 onwards
7geese-cli me export --since 2025-01-01 --output 2025-data.json
```

If you haven't run `auth login` or `sync` yet, `me export` will prompt you to do both automatically.

The JSON output contains:

- **objectives** — OKRs where you are an owner or stakeholder, with key results, progress, and last check-in
- **oneonones** — Your 1:1 meeting history with questions and answers from both participants
- **recognitions** — Recognition badges you sent or received
- **reviews** — Completed performance review snapshots with your answers and your manager's responses, including peer feedback

### `doctor`

Checks that authentication and connectivity are working.

```bash
7geese-cli doctor
```

## Options

| Flag | Description |
|------|-------------|
| `--output <file>` | Write JSON to a file instead of stdout |
| `--since <date>` | Only include data on or after this date (format: `YYYY-MM-DD`). For objectives: always includes open ones; for closed ones, filters by due date. |
| `--config <path>` | Use a custom config file path |

## Authentication details

7Geese uses Okta SSO — there are no API tokens. `auth login` reads your `sgsession4` cookie from Chrome or Firefox's local encrypted store using [sweetcookie](https://github.com/steipete/sweetcookie). Your session stays local; nothing is sent anywhere except back to 7Geese.

If your session expires, just run `auth login` again.

Config is stored at `~/.config/7geese-cli/config.json`.

### CI / headless environments

If you can't run `auth login` (no Chrome, no keychain), set the session cookie directly via environment variable instead:

```bash
export SEVENGEESE_SESSION=<your-sgsession4-cookie-value>
7geese-cli sync
```

To get the cookie value: open Chrome or Firefox, log into app.7geese.com, open DevTools → Application/Storage → Cookies → `app.7geese.com`, and copy the value of `sgsession4`.

## Troubleshooting

**macOS Keychain prompt** — Click Allow when macOS asks for keychain access during `auth login`. If you dismissed it, run `auth login` again.

**`auth login` finds no cookies** — Make sure you're logged into app.7geese.com via Okta in Chrome or Firefox, then retry. Pass `--chrome` or `--firefox` to target a specific browser.

**Linux: `auth login` fails with keyring error** — Make sure a secret service is running (`gnome-keyring-daemon` on GNOME, or KWallet on KDE). Headless/server environments without a secret service are not supported.

**Windows: `auth login` fails to decrypt cookies** — Newer Chromium versions on Windows use app-bound encryption that may not be supported. Try using Firefox instead (`7geese-cli auth login --firefox`).

**401 errors after sync** — Session expired. Run `7geese-cli auth login` again.

**Empty objectives after sync** — Run `7geese-cli doctor` to confirm auth is working. The sync fetches objectives where you are an owner or stakeholder; if you have none in 7Geese, the export will be empty.
