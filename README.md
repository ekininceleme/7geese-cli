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
# 1. Log in using your existing Chrome session (must be logged into app.7geese.com)
7geese-cli auth login

# 2. Sync your data to a local database
7geese-cli sync

# 3. Export everything to JSON
7geese-cli me export --output my-data.json
```

## Commands

### `auth login`

Reads your `sgsession4` session cookie directly from Chrome's encrypted cookie store — no API key or password needed. You must already be logged into 7Geese in Chrome.

**macOS**: the OS will show a dialog asking permission to access the login keychain. Click **Allow** (or **Always Allow** to avoid the prompt in future).

```bash
7geese-cli auth login
```

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

# Print to stdout
7geese-cli me export

# Only include data from 2025 onwards
7geese-cli me export --since 2025-01-01 --output 2025-data.json
```

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

7Geese uses Okta SSO — there are no API tokens. `auth login` reads your `sgsession4` cookie from Chrome's local encrypted store using [sweetcookie](https://github.com/steipete/sweetcookie). Your session stays local; nothing is sent anywhere except back to 7Geese.

If your session expires, just run `auth login` again.

Config is stored at `~/.config/7geese-cli/config.json`.

## Troubleshooting

**macOS Keychain prompt** — Click Allow when macOS asks for keychain access during `auth login`. If you dismissed it, run `auth login` again.

**`auth login` finds no cookies** — Open Chrome, log into app.7geese.com via Okta, then retry.

**Linux: `auth login` fails with keyring error** — Make sure a secret service is running (`gnome-keyring-daemon` on GNOME, or KWallet on KDE). Headless/server environments without a secret service are not supported.

**Windows: `auth login` fails to decrypt cookies** — Newer Chromium versions on Windows use app-bound encryption that may not be supported. Try using Firefox instead (`7geese-cli auth login --firefox`).

**401 errors after sync** — Session expired. Run `7geese-cli auth login` again.

**Empty objectives after sync** — Run `7geese-cli doctor` to confirm auth is working. The sync fetches objectives where you are an owner or stakeholder; if you have none in 7Geese, the export will be empty.
