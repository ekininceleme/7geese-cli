---
name: pp-7geese
description: "The first CLI for 7Geese — log in with Chrome, query OKRs, check-ins, and 1:1s from the terminal. Trigger phrases: `check my okrs`, `7geese status`, `how are my goals`, `team check-in summary`, `prep for my 1:1`, `use 7geese`, `run 7geese`."
author: "Ekin Inceleme"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - 7geese-pp-cli
---

# 7Geese — Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `7geese-pp-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer:
   ```bash
   npx -y @mvanhorn/printing-press install 7geese --cli-only
   ```
2. Verify: `7geese-pp-cli --version`
3. Ensure `$GOPATH/bin` (or `$HOME/go/bin`) is on `$PATH`.

If the `npx` install fails before this CLI has a public-library category, install Node or use the category-specific Go fallback after publish.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

7Geese has no API tokens and no CLI. This tool reads your existing browser session via kooky, syncs your org's performance data to a local SQLite store, and makes every OKR, check-in, 1:1, and recognition queryable offline with JSON output.

## When to Use This CLI

Use this CLI when you need to script 7Geese data, build reporting pipelines, prep for 1:1s with a pre-built brief, or give an AI agent context about your team's OKR health and check-in cadence.

## Unique Capabilities

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

## Command Reference

**badges** — Available badge types for recognition

- `7geese-pp-cli badges` — 

**categories** — Objective categories for tagging

- `7geese-pp-cli categories` — 

**checkins** — Weekly check-ins on goals and progress

- `7geese-pp-cli checkins create` — 
- `7geese-pp-cli checkins get` — 
- `7geese-pp-cli checkins list` — 

**feedbackrequest** — Feedback requests sent to peers

- `7geese-pp-cli feedbackrequest create` — 
- `7geese-pp-cli feedbackrequest list` — 

**notifications** — User notifications

- `7geese-pp-cli notifications` — 

**objectivekeyresults** — Key results belonging to objectives

- `7geese-pp-cli objectivekeyresults create` — 
- `7geese-pp-cli objectivekeyresults get` — 
- `7geese-pp-cli objectivekeyresults list` — 
- `7geese-pp-cli objectivekeyresults update` — 

**objectives** — Personal OKRs and goals

- `7geese-pp-cli objectives create` — 
- `7geese-pp-cli objectives delete` — 
- `7geese-pp-cli objectives get` — 
- `7geese-pp-cli objectives list` — 
- `7geese-pp-cli objectives update` — 

**oneononenotes** — Notes attached to one-on-one meetings

- `7geese-pp-cli oneononenotes` — 

**oneonones** — One-on-one meetings between manager and report

- `7geese-pp-cli oneonones get` — 
- `7geese-pp-cli oneonones list` — 

**organizationalobjectives** — Company-wide OKRs

- `7geese-pp-cli organizationalobjectives get` — 
- `7geese-pp-cli organizationalobjectives list` — 

**peer_feedback** — Peer feedback requests and responses

- `7geese-pp-cli peer_feedback` — 

**performancecycles** — Performance review cycles

- `7geese-pp-cli performancecycles get` — 
- `7geese-pp-cli performancecycles list` — 

**recognitionbadges** — Recognition and kudos sent between users

- `7geese-pp-cli recognitionbadges create` — 
- `7geese-pp-cli recognitionbadges get` — 
- `7geese-pp-cli recognitionbadges list` — 

**team** — Teams in the organization

- `7geese-pp-cli team get` — 
- `7geese-pp-cli team list` — 

**teamobjectives** — Team-level OKRs

- `7geese-pp-cli teamobjectives get` — 
- `7geese-pp-cli teamobjectives list` — 

**user** — Users in the organization

- `7geese-pp-cli user get` — 
- `7geese-pp-cli user list` — 

**userprofile** — Extended user profile with role and manager info

- `7geese-pp-cli userprofile get` — 
- `7geese-pp-cli userprofile list` — 


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
7geese-pp-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match — fall back to `--help` or use a narrower query.

## Recipes


### Pre-1:1 brief

```bash
7geese-pp-cli manager dashboard --agent --select direct_reports.name,direct_reports.okr_health,direct_reports.last_checkin
```

Get a structured brief for all direct reports before a 1:1 meeting

### Find stale OKRs

```bash
7geese-pp-cli objectives stale --days 14 --json | jq '.[].name'
```

List objectives not updated in 2 weeks

### Send kudos

```bash
7geese-pp-cli recognize send --to user@company.com --badge values --message 'Great work on Q2 launch'
```

Send recognition from the terminal

### Check-in history

```bash
7geese-pp-cli checkins list --user me --limit 10 --json
```

See your last 10 check-ins

### OKR health for agents

```bash
7geese-pp-cli okr health --agent --select objectives.name,objectives.progress,objectives.is_overdue
```

Structured OKR health data for AI agent context

## Auth Setup

7Geese uses Okta SSO — there are no API keys. Run `auth login --chrome` once and the CLI reads your `sgsession4` session cookie directly from Chrome's encrypted cookie store using kooky (CGO + Security.framework on macOS). Works with Chrome, Firefox, and Safari. No credentials to manage.

Run `7geese-pp-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable** — JSON on stdout, errors on stderr
- **Filterable** — `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  7geese-pp-cli badges --agent --select id,name,status
  ```
- **Previewable** — `--dry-run` shows the request without sending
- **Offline-friendly** — sync/search commands can use the local SQLite store when available
- **Non-interactive** — never prompts, every input is a flag
- **Explicit retries** — use `--idempotent` only when an already-existing create should count as success, and `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal — piped/agent consumers get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
7geese-pp-cli feedback "the --since flag is inclusive but docs say exclusive"
7geese-pp-cli feedback --stdin < notes.txt
7geese-pp-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.7geese-pp-cli/feedback.jsonl`. They are never POSTed unless `API_7GEESE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `API_7GEESE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
7geese-pp-cli profile save briefing --json
7geese-pp-cli --profile briefing badges
7geese-pp-cli profile list --json
7geese-pp-cli profile show briefing
7geese-pp-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `7geese-pp-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary from this CLI's published public-library entry or pre-built release, then register it:

```bash
claude mcp add 7geese-pp-mcp -- 7geese-pp-mcp
```

Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which 7geese-pp-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   7geese-pp-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `7geese-pp-cli <command> --help`.
