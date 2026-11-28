# How the profile widget works

The profile README shows one image, **the daily code report**: lines added and removed today, total lines of code, the last 30 days, languages, and Claude Code token usage. A GitHub Action rebuilds it and commits it to this repo.

```
   every 4h + 23:53 PT,                          daily on the laptop (launchd)
   and when tokens.json changes                              │
              │                               collector/cmd/report-tokens
   .github/workflows/code-stats.yml             ~/.claude/projects/*.jsonl
              │                                  → tokens per day and type
   collector/ (Go): clone $CODE_STATS_REPOS                  │
     git log  → lines per day                      gh api: PUT stats/tokens.json
     git ls-tree → LOC per language,                         │
       now and at midnight                                   │
     stats/tokens.json → tokens  ◄───────────────────────────┘
              │
   assets/code-report.svg → README.md
```

## Layout

| Path | What it is |
|---|---|
| `collector/` | Go program that collects stats and renders the SVG |
| `collector/tokenlog/` | Reads Claude Code logs and the `tokens.json` record |
| `collector/cmd/report-tokens/` | Runs on the laptop; uploads token totals ([README](../collector/cmd/report-tokens/README.md)) |
| `collector/fonts/` | Barlow Condensed subset inlined into the SVG (SIL OFL) |
| `stats/tokens.json` | Token counts per day, written by `report-tokens` |
| `assets/code-report.svg` | The widget |
| `.github/workflows/code-stats.yml` | Runs the collector and commits the result |

## What the card shows

- **Today**: lines added and removed, commits and files touched today. Before today's first commit it shows the **last day with commits** instead, labelled `LAST ACTIVE · WED 26 NOV`. The token line is that day's total, once `report-tokens` has recorded it.
- **Total**: lines of code now, the change since midnight, repos and all-time commits, and all recorded tokens.
- **30 days**: one bar per day (lines added + removed), today in red, with the window's totals.
- **Languages**: the three largest by lines, the rest folded into "Other" (named beside it).
- **Tokens**: every recorded day, by type, with exact counts and shares. *Cache read* is earlier conversation re-read from the prompt cache on each turn, so it dominates; *cache write* is new context saved to the cache; *output* is Claude's replies and tool calls, thinking included; *input* is uncached prompt text.

No repo names appear on the card, and the collector saves nothing per repo: every number is recomputed from git on each run, and the only file it writes is the card.

## What counts

- **Day** = midnight to now, `America/Los_Angeles`. Every run recomputes the whole day, so running more often never changes what a number means.
- **Commits** on each repo's **default branch** only, by **author date**, excluding merge commits and authors ending in `[bot]`. Other branches are skipped because they hold unmerged work or build output (`gh-pages`). Work on a feature branch shows up once merged, on the day it was written.
- **Lines** come from `git log --numstat`, only for files in a known language (`collector/lang.go`). Lockfiles (`go.sum`, `package-lock.json`, …), `vendor/`, `node_modules/`, `dist/` and generated files (`*.pb.go`, `*_gen.go`, `*.min.js`) are ignored.
- **LOC** = non-blank lines in tracked files on the default branch, using the same language and exclusion rules.
- **`↑ N TODAY`** under the total = LOC now minus LOC as the default branch stood at midnight (its last commit before then, by commit date). Because it's counted from git each time, a repo just added to the list contributes only today's change.
- **Tokens** come from Claude Code's session logs on the laptop, one count per API response (de-duplicated by request ID). See the [report-tokens README](../collector/cmd/report-tokens/README.md).

## Schedule

The workflow runs every 4 hours (minute 17 UTC), plus at 23:53 Pacific so each day's final numbers are captured. There are two cron lines because GitHub cron is UTC and Pacific shifts with daylight saving. GitHub may start scheduled runs a few minutes late. It also runs whenever `stats/tokens.json` changes on `main`.

`report-tokens` runs daily at 23:30 on the laptop, so a day's tokens appear that night, just before the 23:53 run.

The bot commits to `main` on every run. **Pull before you push** to this repo.

## Which repos are counted

The list lives in the Actions **secret** `CODE_STATS_REPOS`, not in a file, so this public repo never names the repos it counts. It's a secret rather than a variable because Actions prints a step's variables at the top of its public log, while secrets show as `***`. Entries are `owner/name`, separated by commas, spaces or newlines. The collector logs repos by number ("repo 2 of 3") and tells Actions to mask every name, so they don't appear in the run logs either, even in a clone error.

Every repo on the list is counted the same way, public or private: making a repo private later needs no change here. Private ones are cloned with the Actions secret `CODE_STATS_TOKEN`: a fine-grained token with **Contents: Read-only**, set here to all of the owner's repos so any repo can be added without touching the token. The collector passes it to `git clone` as an auth header through git's environment config, so it never appears in the command line or logs. Without the token, or if GitHub rejects it (expired, revoked), public repos are cloned without it and still counted; only private ones fail, and the run stops with an error rather than show a card missing a repo. **Renew the token before it expires.**

## Common tasks

**Change the repos**: a secret can't be read back, so set the whole list each time.

```sh
gh secret set CODE_STATS_REPOS --body "Tianbo-Qiu/wheels,Tianbo-Qiu/noosphere,Tianbo-Qiu/whoopctl"
```

The card picks it up on the next run.

**Update now**: Actions tab → *code stats* → *Run workflow*.

**Run locally**:

```sh
cd collector
export CODE_STATS_REPOS="Tianbo-Qiu/wheels,Tianbo-Qiu/noosphere,Tianbo-Qiu/whoopctl"
go run . -root ..
```

This rewrites `assets/code-report.svg` in the repo, just like CI. Private repos need `CODE_STATS_TOKEN` in the environment (`export CODE_STATS_TOKEN="$(gh auth token)"` works for your own repos).

**Preview a different time** (e.g. a quiet day) without touching the real card: point `-root` at a scratch copy.

```sh
mkdir -p /tmp/preview && cp -r stats /tmp/preview/
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
- **Late commits**: a commit pushed days after it was written appears on the day it was written (author date) in the 30-day chart, but counts toward `↑ N TODAY` on the day it reached the branch.
- **Tokens from one machine**: only the laptop running `report-tokens` is counted.
