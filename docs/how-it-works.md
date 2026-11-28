# How the profile widget works

The profile README shows one image, **the daily code report**: lines added and removed today, total lines of code, the last 30 days, languages, and Claude Code token usage. A GitHub Action rebuilds it and commits it to this repo.

```
   every 4h + 23:53 PT,                          daily on the laptop (launchd)
   and when tokens.json changes                              │
              │                               collector/cmd/report-tokens
   .github/workflows/code-stats.yml             ~/.claude/projects/*.jsonl
              │                                  → tokens per day and type
   collector/ (Go): clone repos.txt                          │
     git log  → lines per day                      gh api: PUT stats/tokens.json
     ls-files → LOC per language                             │
     stats/tokens.json → tokens  ◄───────────────────────────┘
              │
   stats/YYYY-MM-DD.json   assets/code-report.svg
              │
          README.md
```

## Layout

| Path | What it is |
|---|---|
| `collector/` | Go program that collects stats and renders the SVG |
| `collector/repos.txt` | The repos that are counted, one `owner/name` per line |
| `collector/tokenlog/` | Reads Claude Code logs and the `tokens.json` record |
| `collector/cmd/report-tokens/` | Runs on the laptop; uploads token totals ([README](../collector/cmd/report-tokens/README.md)) |
| `collector/fonts/` | Barlow Condensed subset inlined into the SVG (SIL OFL) |
| `stats/YYYY-MM-DD.json` | One snapshot per day. Needed for the LOC change under the total |
| `stats/tokens.json` | Token counts per day, written by `report-tokens` |
| `assets/code-report.svg` | The widget |
| `.github/workflows/code-stats.yml` | Runs the collector and commits the result |

## What the card shows

- **Today**: lines added and removed, commits and files touched today. Before today's first commit it shows the **last day with commits** instead, labelled `LAST ACTIVE · WED 26 NOV`. The token line is that day's total, once `report-tokens` has recorded it.
- **Total**: lines of code now, the change since the previous snapshot, repos and all-time commits, and all recorded tokens.
- **30 days**: one bar per day (lines added + removed), today in red, with the window's totals.
- **Languages**: the three largest by lines, the rest folded into "Other" (named beside it).
- **Tokens**: every recorded day, by type, with exact counts and shares. *Cache read* is earlier conversation re-read from the prompt cache on each turn, so it dominates; *cache write* is new context saved to the cache; *output* is Claude's replies and tool calls, thinking included; *input* is uncached prompt text.

No repo names appear on the card, so tracking a private repo shows only its numbers.

## What counts

- **Day** = midnight to now, `America/Los_Angeles`. Every run recomputes the whole day, so running more often never changes what a number means.
- **Commits** on each repo's **default branch** only, by **author date**, excluding merge commits and authors ending in `[bot]`. Other branches are skipped because they hold unmerged work or build output (`gh-pages`). Work on a feature branch shows up once merged, on the day it was written.
- **Lines** come from `git log --numstat`, only for files in a known language (`collector/lang.go`). Lockfiles (`go.sum`, `package-lock.json`, …), `vendor/`, `node_modules/`, `dist/` and generated files (`*.pb.go`, `*_gen.go`, `*.min.js`) are ignored.
- **LOC** = non-blank lines in tracked files on the default branch, using the same language and exclusion rules.
- **`↑ N TODAY`** under the total = LOC minus the LOC in the latest earlier snapshot in `stats/`, over repos present in both. A repo added to `repos.txt` doesn't count toward it on its first day. On the very first run there's nothing to compare against, so it's left out.
- **Tokens** come from Claude Code's session logs on the laptop, one count per API response (de-duplicated by request ID). See the [report-tokens README](../collector/cmd/report-tokens/README.md).

## Schedule

The workflow runs every 4 hours (minute 17 UTC), plus at 23:53 Pacific so each day's final numbers are captured. There are two cron lines because GitHub cron is UTC and Pacific shifts with daylight saving. GitHub may start scheduled runs a few minutes late. It also runs whenever `stats/tokens.json` changes on `main`.

`report-tokens` runs daily at 23:30 on the laptop, so a day's tokens appear that night, just before the 23:53 run.

The bot commits to `main` on every run. **Pull before you push** to this repo.

## Private repos

Cloning a private repo needs a token. Create a fine-grained token with **Contents: Read-only** on the private repos in `repos.txt` and save it in this repo as the Actions secret `CODE_STATS_TOKEN`. The collector passes it to `git clone` as an auth header through git's environment config, so it never appears in the command line or logs. Without the secret, public repos still work.

## Common tasks

**Add or remove a repo**: edit `collector/repos.txt`.

**Update now**: Actions tab → *code stats* → *Run workflow*.

**Run locally**:

```sh
cd collector
go run . -root ..
```

This writes today's snapshot and `assets/code-report.svg` into the repo, just like CI. Private repos need `CODE_STATS_TOKEN` in the environment.

**Preview a different time** (e.g. a quiet day) without touching real data: point `-root` at a scratch copy.

```sh
mkdir -p /tmp/preview/collector && cp collector/repos.txt /tmp/preview/collector/
cp -r stats /tmp/preview/
(cd collector && go run . -root /tmp/preview -now 2026-10-19T10:17:00-07:00)
open /tmp/preview/assets/code-report.svg
```

**Change what counts as code**: `collector/lang.go` (`langByExt`, `skipNames`, `skipDirs`, `skipSuffixes`).

**Change the look**: `collector/render.go`. The AI logo row is `collector/logos.go`.

**Tests**: `cd collector && go test ./...`

### Optional: update right after a push

Instead of waiting up to 4 hours, a tracked repo can ping this workflow on every push to its default branch:

1. Create a fine-grained token for `Tianbo-Qiu/Tianbo-Qiu` with **Contents: Read and write** (needed to send `repository_dispatch`).
2. In each tracked repo, add it as the secret `PROFILE_DISPATCH_TOKEN` and add this workflow:

```yaml
name: notify profile
on:
  push:
    branches: [main]
jobs:
  notify:
    runs-on: ubuntu-latest
    steps:
      - run: |
          curl -fsS -X POST \
            -H "Authorization: Bearer ${{ secrets.PROFILE_DISPATCH_TOKEN }}" \
            -H "Accept: application/vnd.github+json" \
            https://api.github.com/repos/Tianbo-Qiu/Tianbo-Qiu/dispatches \
            -d '{"event_type":"code-push"}'
```

## Known limits

- **Fonts**: an SVG shown as an image can't load web fonts. The condensed figures use a Barlow Condensed subset inlined in the SVG; small labels use the viewer's monospace font (SF Mono / Menlo / Consolas).
- **Theme**: one light card for both GitHub themes; it reads as a printed sheet on a dark page.
- **Motion**: digits roll up and bars grow when the image loads, and the red dot pulses. Viewers with reduced motion get the final state at once.
- **Late commits**: rebasing or pushing old commits later puts them back on the day they were written. That past day's snapshot won't be rewritten, so its numbers can be slightly low.
- **Tokens from one machine**: only the laptop running `report-tokens` is counted.
