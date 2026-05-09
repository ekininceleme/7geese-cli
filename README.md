# 7Geese CLI

**The first CLI for 7Geese — log in with Chrome, query OKRs, check-ins, and 1:1s from the terminal.**

7Geese has no API tokens and no CLI. This tool reads your existing browser session via sweetcookie, syncs your org's performance data to a local SQLite store, and makes every OKR, check-in, 1:1, and recognition queryable offline with JSON output.

## Install

The recommended path installs both the `7geese-pp-cli` binary and the `pp-7geese` agent skill in one shot:

```bash
npx -y @mvanhorn/printing-press install 7geese
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press install 7geese --cli-only
```


### Without Node

The generated install path is category-agnostic until this CLI is published. If `npx` is not available before publish, install Node or use the category-specific Go fallback from the public-library entry after publish.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/7geese-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-7geese --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-7geese --force
```

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

```
Install the pp-7geese skill from https://github.com/mvanhorn/printing-press-library/tree/main/cli-skills/pp-7geese. The skill defines how its required CLI can be installed.
```

## Authentication

7Geese uses Okta SSO — there are no API keys. Run `auth login --chrome` once and the CLI reads your `sgsession4` session cookie directly from Chrome's encrypted cookie store using sweetcookie. Works with Chrome, Firefox, and Safari. No credentials to manage.

## Quick Start

```bash
# Read your session from Chrome (must be logged into app.7geese.com)
7geese-pp-cli auth login --chrome


# Pull objectives, check-ins, users into local SQLite
7geese-pp-cli sync


# List your personal OKRs
7geese-pp-cli objectives list --json


# See which OKRs are on track vs stale
7geese-pp-cli okr health


# Pre-1:1 brief for your direct reports
7geese-pp-cli manager dashboard

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Zero-friction auth

- **`auth login --chrome`** — Log in by reading your existing Chrome (or Firefox/Safari) session — no API key needed.

  _Enables zero-credential-management auth for any user already logged into 7Geese in their browser._

  ```bash
  7geese-pp-cli auth login --chrome
  ```

### Local state that compounds

- **`okr health`** — See which objectives are on track, at risk, or stale — across personal, team, and org levels.

  _Use before 1:1s or team meetings to quickly identify which OKRs need attention._

  ```bash
  7geese-pp-cli okr health --json
  ```
- **`objectives stale`** — Find objectives that have not been updated in N days.

  _Identify OKRs at risk of being abandoned before performance cycle closes._

  ```bash
  7geese-pp-cli objectives stale --days 14 --json
  ```
- **`checkins streak`** — See consecutive weekly check-in streaks for yourself or your team.

  _Track team engagement habit formation over time._

  ```bash
  7geese-pp-cli checkins streak --user me --agent
  ```
- **`recognize leaderboard`** — See who gives and receives the most recognition this month.

  _Surface recognition patterns for culture reporting and manager coaching._

  ```bash
  7geese-pp-cli recognize leaderboard --period month --json
  ```

### Agent-native plumbing

- **`manager dashboard`** — Pre-1:1 brief: direct reports, their recent check-ins, OKR health, and upcoming meetings in one view.

  _Prepare for 1:1s in seconds instead of clicking through multiple 7Geese screens._

  ```bash
  7geese-pp-cli manager dashboard --json
  ```
- **`me week`** — Everything relevant to you this week: check-ins due, OKRs to update, upcoming 1:1s.

  _Morning standup context in one command; useful as an agent briefing tool._

  ```bash
  7geese-pp-cli me week --agent
  ```

## Usage

Run `7geese-pp-cli --help` for the full command reference and flag list.

## Commands

### badges

Available badge types for recognition

- **`7geese-pp-cli badges list`** - 

### categories

Objective categories for tagging

- **`7geese-pp-cli categories list`** - 

### checkins

Weekly check-ins on goals and progress

- **`7geese-pp-cli checkins create`** - 
- **`7geese-pp-cli checkins get`** - 
- **`7geese-pp-cli checkins list`** - 

### feedbackrequest

Feedback requests sent to peers

- **`7geese-pp-cli feedbackrequest create`** - 
- **`7geese-pp-cli feedbackrequest list`** - 

### notifications

User notifications

- **`7geese-pp-cli notifications list`** - 

### objectivekeyresults

Key results belonging to objectives

- **`7geese-pp-cli objectivekeyresults create`** - 
- **`7geese-pp-cli objectivekeyresults get`** - 
- **`7geese-pp-cli objectivekeyresults list`** - 
- **`7geese-pp-cli objectivekeyresults update`** - 

### objectives

Personal OKRs and goals

- **`7geese-pp-cli objectives create`** - 
- **`7geese-pp-cli objectives delete`** - 
- **`7geese-pp-cli objectives get`** - 
- **`7geese-pp-cli objectives list`** - 
- **`7geese-pp-cli objectives update`** - 

### oneononenotes

Notes attached to one-on-one meetings

- **`7geese-pp-cli oneononenotes list`** - 

### oneonones

One-on-one meetings between manager and report

- **`7geese-pp-cli oneonones get`** - 
- **`7geese-pp-cli oneonones list`** - 

### organizationalobjectives

Company-wide OKRs

- **`7geese-pp-cli organizationalobjectives get`** - 
- **`7geese-pp-cli organizationalobjectives list`** - 

### peer_feedback

Peer feedback requests and responses

- **`7geese-pp-cli peer_feedback list`** - 

### performancecycles

Performance review cycles

- **`7geese-pp-cli performancecycles get`** - 
- **`7geese-pp-cli performancecycles list`** - 

### recognitionbadges

Recognition and kudos sent between users

- **`7geese-pp-cli recognitionbadges create`** - 
- **`7geese-pp-cli recognitionbadges get`** - 
- **`7geese-pp-cli recognitionbadges list`** - 

### team

Teams in the organization

- **`7geese-pp-cli team get`** - 
- **`7geese-pp-cli team list`** - 

### teamobjectives

Team-level OKRs

- **`7geese-pp-cli teamobjectives get`** - 
- **`7geese-pp-cli teamobjectives list`** - 

### user

Users in the organization

- **`7geese-pp-cli user get`** - 
- **`7geese-pp-cli user list`** - 

### userprofile

Extended user profile with role and manager info

- **`7geese-pp-cli userprofile get`** - 
- **`7geese-pp-cli userprofile list`** - 


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
7geese-pp-cli badges

# JSON for scripting and agents
7geese-pp-cli badges --json

# Filter to specific fields
7geese-pp-cli badges --json --select id,name,status

# Dry run — show the request without sending
7geese-pp-cli badges --dry-run

# Agent mode — JSON + compact + no prompts in one flag
7geese-pp-cli badges --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Use with Claude Code

Install the focused skill — it auto-installs the CLI on first invocation:

```bash
npx skills add mvanhorn/printing-press-library/cli-skills/pp-7geese -g
```

Then invoke `/pp-7geese <query>` in Claude Code. The skill is the most efficient path — Claude Code drives the CLI directly without an MCP server in the middle.

<details>
<summary>Use as an MCP server in Claude Code (advanced)</summary>

If you'd rather register this CLI as an MCP server in Claude Code, install the MCP binary first:


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Then register it:

```bash
# Some tools work without auth. For full access, set up auth first:
7geese-pp-cli auth login --chrome

claude mcp add 7geese 7geese-pp-mcp
```

</details>

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

The bundle reuses your local browser session — set it up first if you haven't:

```bash
7geese-pp-cli auth login --chrome
```

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/7geese-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "7geese": {
      "command": "7geese-pp-mcp"
    }
  }
}
```

</details>

## Health Check

```bash
7geese-pp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: ``

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SEVENGEESE_SESSION` | per_call | Yes | Set to your API credential. |

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `7geese-pp-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SEVENGEESE_SESSION`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **auth login --chrome: no cookies found** — Open Chrome, log into app.7geese.com via Okta, then retry
- **401 Unauthorized on API calls** — Session expired — run `auth login --chrome` again to refresh
- **Empty results after sync** — Check `doctor` output; ensure your org has objectives created in 7Geese

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**7geese-python-api-example**](https://github.com/7Geese/7geese-python-api-example) — Python
- [**7geese-nodejs-api-example**](https://github.com/7Geese/7geese-nodejs-api-example) — JavaScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
