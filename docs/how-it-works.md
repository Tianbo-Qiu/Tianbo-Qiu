# How the profile widget works

The profile README shows two images:

1. **Code activity**: today's lines added/removed per repo plus total LOC per repo. It's rebuilt by a GitHub Action and committed to this repo.
2. **Visitor count**: a one-line SVG served live by a Cloudflare Worker.

```
             every 4h + 23:53 PT                       on every profile view
                     │                                          │
       .github/workflows/code-stats.yml          GitHub image proxy (camo)
                     │                                          │
        collector/ (Go): clone repos.txt          worker/ (Cloudflare Worker)
          git log  → today's +/−                     D1: n = n + 1
          ls-files → LOC per language                 → "1,204 visitors"
                     │
     stats/YYYY-MM-DD.json   assets/code-stats-{dark,light}.svg
                     │
                 README.md  ◄────────────────────────────────┘
```

## Layout

| Path | What it is |
|---|---|
| `collector/` | Go program that collects stats and renders the SVGs |
| `collector/repos.txt` | The repos that are counted, one `owner/name` per line |
| `stats/` | One JSON snapshot per day. Needed for the `↑ +N` deltas |
| `assets/code-stats-*.svg` | The widget, in dark and light |
| `.github/workflows/code-stats.yml` | Runs the collector and commits the result |
| `worker/` | Visitor counter (Cloudflare Worker + D1) |

## Code activity

### What counts

- **Day** = midnight to now, `America/Los_Angeles`. Every run recomputes the whole day, so running more often never changes what a number means.
- **Commits** on each repo's **default branch** only, by **author date**, excluding merge commits and authors ending in `[bot]`. Other branches are skipped because they hold unmerged work or build output (`gh-pages`). Work on a feature branch shows up once merged, on the day it was written.
- **Lines** come from `git log --numstat`, only for files in a known language (`collector/lang.go`). Lockfiles (`go.sum`, `package-lock.json`, …), `vendor/`, `node_modules/`, `dist/` and generated files (`*.pb.go`, `*_gen.go`, `*.min.js`) are ignored.
- **LOC** = non-blank lines in tracked files on the default branch, using the same language and exclusion rules.
- **`↑ +N` / `↓ −N` / `→ 0`** = today's LOC minus the LOC in the latest earlier snapshot in `stats/`. On the very first run there is nothing to compare against, so it shows `→ 0`.

### Quiet days

With no commits today, the big numbers are dimmed and a box says "Quiet day. No commits yet." with the most recent day that had commits and the busiest repo on that day.

### Schedule

The workflow runs every 4 hours (minute 17 UTC), plus at 23:53 Pacific so each day's final numbers are captured. There are two cron lines because GitHub cron is UTC and Pacific shifts with daylight saving. GitHub may start scheduled runs a few minutes late.

It commits only when `stats/` or `assets/` changed. The "updated" timestamp changes on every run, so in practice that's every run. **Pull before you push** to this repo, since the bot commits to `main`.

No secrets are needed: the tracked repos are public, and the built-in `GITHUB_TOKEN` can push to this repo. To track a **private** repo you'd need a fine-grained token with Contents: read on it, saved as a secret and used for the clone.

### Common tasks

**Add or remove a repo**: edit `collector/repos.txt`. A newly added repo shows its full LOC as `↑` on its first day.

**Update now**: Actions tab → *code stats* → *Run workflow*.

**Run locally**:

```sh
cd collector
go run . -root ..
```

This writes today's snapshot and both SVGs into the repo, just like CI.

**Preview a different time** (e.g. a quiet day) without touching real data: point `-root` at a scratch copy.

```sh
mkdir -p /tmp/preview/collector && cp collector/repos.txt /tmp/preview/collector/
cp -r stats /tmp/preview/
(cd collector && go run . -root /tmp/preview -now 2026-10-19T10:17:00-07:00)
open /tmp/preview/assets/code-stats-dark.svg
```

**Change what counts as code**: `collector/lang.go` (`langByExt`, `skipNames`, `skipDirs`, `skipSuffixes`).

**Change the look**: `collector/render.go`. Colors are GitHub's palette, in `darkTheme` / `lightTheme`.

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

## Visitor counter

`worker/src/index.js` handles `GET /views.svg`: it adds 1 in D1 and returns the new count as an SVG. Deployed at `https://profile-views.tianbo-dev.workers.dev/views.svg`.

- **Atomic**: the increment is a single SQL upsert (`ON CONFLICT … SET n = n + 1 RETURNING n`), so concurrent views can't overwrite each other.
- **Theme**: `?theme=dark` / `?theme=light` fixes the text color, and the README picks one with `<picture>`, the same way as the widget. Without the parameter the SVG follows the OS setting.
- **No caching**: the response sends `no-cache`, so GitHub's image proxy fetches a fresh count each time.
- **Approximate**: every view goes through GitHub's image proxy, so there's no visitor IP. Your own views, bots and proxy retries all count. Anyone can inflate it by requesting the URL.
- **Cost**: free plan (100k requests/day, 100k D1 writes/day). Past the limit the image breaks until 00:00 UTC. Nothing is billed.

### Common tasks

Run these from `worker/` (needs `wrangler login`):

```sh
wrangler deploy                      # ship changes to src/index.js
wrangler dev                         # local test at http://localhost:8787/views.svg
                                     # (first time: wrangler d1 execute profile-views --local --file schema.sql)

# read or set the count
wrangler d1 execute profile-views --remote --command "SELECT n FROM counter"
wrangler d1 execute profile-views --remote --command "UPDATE counter SET n = 1000 WHERE id = 'profile'"
```

The D1 database id in `wrangler.toml` is not a secret; access needs your Cloudflare login.

## Known limits

- **Fonts**: an SVG in an `<img>` can't load web fonts, so the widget uses the viewer's monospace font (JetBrains Mono if installed, else SF Mono / Menlo / Consolas). Numbers are right-aligned, so columns line up with any monospace font.
- **Theme**: both README images use `<picture>` with `prefers-color-scheme`, which GitHub matches to the viewer's GitHub theme (Settings → Appearance).
- **Late commits**: rebasing or pushing old commits later puts them back on the day they were written. That past day's snapshot won't be rewritten, so its numbers can be slightly low.
