# report-tokens

Totals Claude Code token usage per day from this machine's session logs
(`~/.claude/projects/**/*.jsonl`) and writes it to `stats/tokens.json` in the
profile repo, which triggers the widget to re-render.

- Each API response is counted once (by request ID), split by type: input,
  output (thinking included), cache write, cache read.
- Days are Pacific time, like the rest of the widget.
- Only these daily totals are uploaded: no project names, paths or prompts.
- Claude Code deletes old logs after about 30 days. Days already in
  `tokens.json` keep their numbers, so the total keeps growing.

## Run it

Needs Go and the GitHub CLI, logged in with write access to the profile repo
(`gh auth login`).

```sh
cd collector
go run ./cmd/report-tokens -dry-run   # print the merged file, upload nothing
go run ./cmd/report-tokens            # update stats/tokens.json on GitHub
```

## Daily job (launchd)

Build it once, then load a job that runs at 23:30 every day. If the Mac is
asleep then, launchd runs it on wake.

```sh
cd collector
go build -o ~/.local/bin/report-tokens ./cmd/report-tokens
```

Save as `~/Library/LaunchAgents/com.tianbo.report-tokens.plist`, replacing
`USER` with your macOS user name:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.tianbo.report-tokens</string>
  <key>ProgramArguments</key>
  <array><string>/Users/USER/.local/bin/report-tokens</string></array>
  <key>EnvironmentVariables</key>
  <dict><key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string></dict>
  <key>StartCalendarInterval</key>
  <dict><key>Hour</key><integer>23</integer><key>Minute</key><integer>30</integer></dict>
  <key>StandardOutPath</key><string>/tmp/report-tokens.log</string>
  <key>StandardErrorPath</key><string>/tmp/report-tokens.log</string>
</dict>
</plist>
```

```sh
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.tianbo.report-tokens.plist
launchctl kickstart gui/$(id -u)/com.tianbo.report-tokens   # run once now
tail /tmp/report-tokens.log
```

`PATH` must include the directory holding `gh`. To stop the job:
`launchctl bootout gui/$(id -u)/com.tianbo.report-tokens`.
