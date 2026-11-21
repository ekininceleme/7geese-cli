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
7geese-cli me export > my-data.json
```

Or step by step:

```bash
7geese-cli auth login   # read session from Chrome or Firefox
7geese-cli sync         # pull data into a local database
7geese-cli me export > my-data.json
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

Downloads your data into a local SQLite database at `~/.local/share/7geese-cli/data.db`.

```bash
7geese-cli sync
7geese-cli sync --human-friendly   # text output instead of JSON events
```

Each run is a full refresh:

- Records that changed in 7Geese are updated.
- Records that were deleted in 7Geese are deleted from the local database.
- If one dataset fails, its earlier local data stays, the command names the dataset and the exit code is not 0. Run `sync` again.
- If the session is not valid, `sync` reads a new session from your browser. If that is not possible, it stops before it changes data.

Datasets: user profiles, 1:1 meetings and their notes, objectives with the full check-in history, reviews, recognition (given and received), check-ins, notifications, feedback requests and peer feedback. The objectives and reviews of your direct reports are included.

A database holds the data of one 7Geese user. If you log in as a different user, use `--db` with a different path.

Typed SQL views are available for your own queries: `userprofile_view`, `oneonone_view`, `objective_view`, `review_view`, `recognition_given_view`, `recognition_received_view`, `checkin_view`, `notification_view`.

```bash
sqlite3 ~/.local/share/7geese-cli/data.db "SELECT name, progress FROM objective_view WHERE closed = 0"
```

### `me export`

Exports all your performance data to a single JSON file.

```bash
7geese-cli me export > my-data.json
7geese-cli me export | jq '.me.objectives'

# Only include data from 2025 onwards
7geese-cli me export --since 2025-01-01 > 2025-data.json

# Export a direct report (name, email or numeric ID)
7geese-cli me export --user 123456
```

`me export` reads only the local database. If the local data is not complete, `me export` asks to run `auth login` and `sync`. In a non-interactive context (CI, script) there is no prompt and the command fails with a clear error. Pass `--yes` to confirm automatically:

```bash
7geese-cli me export --yes > my-data.json
```

The JSON output contains:

- **synced_at** — Time of the oldest sync of the exported data. Run `sync` to refresh the data.
- **me.profile** — Name, email, position, hire date and manager
- **me.objectives** — OKRs where you are an owner, a stakeholder or a follower, with key results and the full check-in history
- **me.reviews** — Performance reviews in all workflow states, with your answers, the answers of your manager and peer feedback
- **me.oneonones** — Your 1:1 meetings with the questions, the answers of the two participants and the comments
- **me.recognitions** — Recognition badges that you received and gave
- **me.checkins** — Check-ins that you posted
- **me.notifications** — Your notification feed
- **direct_reports** — Profile, objectives and reviews of each direct report

7Geese gives 1:1 meetings, recognition, check-ins and notifications only for the logged-in user. Thus an export with `--user` contains the profile, the objectives and the reviews only, and it is available only for your direct reports.

A value that 7Geese does not document is exported as `unknown_<n>` (for example an objective type or a meeting status).

### `me week`

Shows the check-ins that you posted this week, your open objectives with no check-in this week, and the 1:1 meetings in the next 7 days. It reads only the local database.

```bash
7geese-cli me week
7geese-cli me week --json
```

### `me schema`

Prints the JSON Schema for the output of `me export`. Useful for tooling and integrations that need to understand the data shape before consuming it.

```bash
7geese-cli me schema
7geese-cli me schema | jq '.["$defs"].exportObjective'
```

The schema is also committed to the repo at [`schema/export.json`](schema/export.json).

### `doctor`

Checks that authentication and connectivity are working.

```bash
7geese-cli doctor
```

## Options

| Flag | Description |
|------|-------------|
| `--since <date>` | `me export`: include only data on or after this date (format: `YYYY-MM-DD`). The period starts at 00:00 local time on this date. 1:1 meetings, recognitions, check-ins and notifications are compared by their timestamp. Reviews are included when the review period ends on or after this date. Open objectives are always included. Closed objectives are included when the due date or the completion date is on or after this date. |
| `--user <name, email or ID>` | `me export`: export a direct report. |
| `--db <path>` | Use a different database file. |
| `--yes` | Auto-confirm all prompts (auth login, sync). Useful for scripts and automation. |
| `--no-input` | Disable all prompts and fail immediately if auth or data is missing. |
| `--config <path>` | Use a custom config file path |

## Authentication details

7Geese uses Okta SSO — there are no API tokens. `auth login` reads your `sgsession4` cookie from Chrome or Firefox's local encrypted store using [kooky](https://github.com/browserutils/kooky). Your session stays local; nothing is sent anywhere except back to 7Geese.

If your session expires, `sync` reads a new session from your browser. If that fails, run `auth login` again.

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

**`sync` says that the session is not valid** — Log into app.7geese.com in Chrome or Firefox, then run `7geese-cli auth login` and `7geese-cli sync` again.

**`sync` says that the database holds the data of a different user** — Use `--db` with a different path, or delete the database file.

**Empty objectives after sync** — Run `7geese-cli doctor` to confirm auth is working. The sync gets objectives where you are an owner, a stakeholder or a follower. If you have none in 7Geese, the list is empty.
