---
name: pp-7geese
description: "CLI for 7Geese: log in with the browser session, sync your performance data to a local SQLite database and export objectives, check-ins, 1:1 meetings, reviews and recognition as JSON. Trigger phrases: `check my okrs`, `7geese status`, `how are my goals`, `export my 7geese data`, `prep for my 1:1`, `use 7geese`, `run 7geese`."
author: "Ekin Inceleme"
license: "Apache-2.0"
argument-hint: "<command> [args]"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - 7geese-cli
---

# 7Geese CLI

7Geese has no API tokens. This CLI reads the session of a browser that is logged in to 7Geese, downloads the data of that user to a local SQLite database and exports it as JSON.

## Prerequisites

Make sure that the CLI is installed before you use a command from this skill:

```bash
7geese-cli version
```

If the command is not found, install the CLI:

```bash
brew tap ekininceleme/tap
brew install sevengeese-cli
```

Pre-built binaries are on the [release page](https://github.com/ekininceleme/7geese-cli/releases/latest).

## Commands

These are all the commands. There are no other commands.

| Command | Function |
|---------|----------|
| `7geese-cli auth login` | Read the 7Geese session from Chrome or Firefox (`--chrome`, `--firefox`). |
| `7geese-cli auth status` | Show the authentication status. |
| `7geese-cli auth logout` | Remove the stored session. |
| `7geese-cli sync` | Download all data and replace the local copy. |
| `7geese-cli me export` | Write one JSON document with all data of the user. |
| `7geese-cli me week` | Show the check-ins of this week, the objectives to update and the 1:1 meetings of the next 7 days. |
| `7geese-cli me schema` | Print the JSON Schema of the `me export` output. |
| `7geese-cli doctor` | Check the authentication, the connection and the age of the local data. |
| `7geese-cli version` | Print the version. |

### `sync`

Each run is a full refresh. Records that were changed in 7Geese are updated. Records that were deleted in 7Geese are deleted from the local database.

Datasets: user profiles, 1:1 meetings and their notes, objectives with the full check-in history, reviews, the feedback requests about the user with their responses, recognition (given and received), check-ins and notifications. The objectives and reviews of the direct reports are included.

If a dataset fails, its earlier local data stays and the exit code is not 0. Run the command again.

The output is one JSON event for each dataset on stdout and a final `sync_summary` event. Use `--human-friendly` for text output.

### `me export`

```bash
7geese-cli me export --yes --output my-data.json
7geese-cli me export --yes --since 2026-01-01
7geese-cli me export --yes --user "Name of a direct report"
7geese-cli me export --yes | jq '.me.objectives'
```

- `me` has the sections `profile`, `objectives`, `reviews`, `feedback`, `oneonones`, `recognitions`, `checkins` and `notifications`.
- `feedback` has the feedback requests about the user with the responses. Each review has `feedback_request_ids` for its attached requests. A request with `"visible": false` is attached to a review but 7Geese does not show it to the user: only `id` and `title` are available, and the sync gives a `sync_warning` event for it.
- `direct_reports` has the profile, the objectives and the reviews of each direct report.
- `synced_at` shows the age of the local data. Run `sync` to refresh the data.
- `--user` works only for direct reports. The sections `feedback`, `oneonones`, `recognitions`, `checkins` and `notifications` are available only for the session user.
- `--since` starts the period at 00:00 local time on the given date.
- If the local data is not complete, the command asks to run `sync`. Use `--yes` to accept without a prompt.

### `me week`

```bash
7geese-cli me week
7geese-cli me week --json
```

The command reads only the local database. Run `sync` first.

## SQL access

The database is at `~/.local/share/7geese-cli/data.db`. Use `--db <path>` to select a different file. One database holds the data of one 7Geese user.

Typed views: `userprofile_view`, `oneonone_view`, `objective_view`, `review_view`, `recognition_given_view`, `recognition_received_view`, `checkin_view`, `notification_view`, `feedback_request_view`, `feedback_answer_view`.

```bash
sqlite3 ~/.local/share/7geese-cli/data.db "SELECT name, progress FROM objective_view WHERE closed = 0"
```

## Recipes

### Status of the open objectives

```bash
7geese-cli sync && 7geese-cli me export --yes | jq '.me.objectives[] | select(.closed == false) | {name, progress, due_date}'
```

### Preparation for a 1:1 meeting

```bash
7geese-cli me week --json
7geese-cli me export --yes --since 2026-01-01 | jq '.me.oneonones[:3]'
```

### Data of a direct report

```bash
7geese-cli me export --yes | jq '.direct_reports[] | {name: .profile.name, objectives: [.objectives[].name]}'
```

Use `7geese-cli me schema` to see all fields.

## Authentication

7Geese uses SSO. The user must be logged in to 7Geese in Chrome or Firefox. `auth login` reads the `sgsession4` cookie from the cookie store of the browser. On macOS, the system asks for access to the login keychain.

If the session is not valid, `sync` reads a new session from the browser. If that is not possible, it stops before it changes data. Then run `7geese-cli auth login`.

## Errors

The exit code is 0 for success. All other exit codes are failures, and the error text is on stderr. Do not use partial output of a command that failed.

## Argument parsing

Parse `$ARGUMENTS`:

1. Empty, `help` or `--help`: show the output of `7geese-cli --help`.
2. All other values: run them as a CLI command. Add `--yes` to `me export` so that the command does not wait for a prompt.
