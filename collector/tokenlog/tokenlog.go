// Package tokenlog totals Claude Code token usage per day and keeps the
// running record in stats/tokens.json.
//
// Claude Code writes one JSONL log per session under ~/.claude/projects. Each
// API response appears as an "assistant" line carrying a usage object, and a
// response split into several content blocks repeats the same usage on each
// line, so lines are de-duplicated by request ID. Claude Code deletes old
// logs (after 30 days by default), which is why the totals are kept in a file
// rather than recomputed from the logs every time.
package tokenlog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Counts holds token counts by type, named as in the API's usage object.
type Counts struct {
	Input      int64 `json:"input"`       // input_tokens: prompt tokens not read from cache
	Output     int64 `json:"output"`      // output_tokens: replies and tool calls, thinking included
	CacheWrite int64 `json:"cache_write"` // cache_creation_input_tokens
	CacheRead  int64 `json:"cache_read"`  // cache_read_input_tokens
}

func (c Counts) Total() int64 { return c.Input + c.Output + c.CacheWrite + c.CacheRead }

func (c *Counts) add(o Counts) {
	c.Input += o.Input
	c.Output += o.Output
	c.CacheWrite += o.CacheWrite
	c.CacheRead += o.CacheRead
}

// File is the stats/tokens.json record: token counts per day (YYYY-MM-DD in
// the widget's timezone).
type File struct {
	UpdatedAt time.Time         `json:"updated_at"`
	Days      map[string]Counts `json:"days"`
}

// Parse decodes a tokens.json document.
func Parse(data []byte) (*File, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Days == nil {
		f.Days = map[string]Counts{}
	}
	return &f, nil
}

// Load reads a tokens.json file. A missing file returns nil and no error.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Marshal encodes f with days in date order, ending in a newline.
func (f *File) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Merge folds freshly scanned day counts into f and reports whether anything
// changed. A day takes the fresh counts unless they total less than what is
// recorded: logs only grow, so a smaller total means some of that day's logs
// were deleted and the recorded numbers are the better ones.
func (f *File) Merge(fresh map[string]Counts) bool {
	if f.Days == nil {
		f.Days = map[string]Counts{}
	}
	changed := false
	for day, c := range fresh {
		old, ok := f.Days[day]
		if ok && (c == old || c.Total() < old.Total()) {
			continue
		}
		f.Days[day] = c
		changed = true
	}
	return changed
}

// Total sums every recorded day.
func (f *File) Total() Counts {
	var t Counts
	for _, c := range f.Days {
		t.add(c)
	}
	return t
}

// Since returns the earliest recorded day, or "" when there are none.
func (f *File) Since() string {
	days := make([]string, 0, len(f.Days))
	for d := range f.Days {
		days = append(days, d)
	}
	if len(days) == 0 {
		return ""
	}
	sort.Strings(days)
	return days[0]
}

// logLine is the part of a Claude Code log line we read.
type logLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"requestId"`
	Message   struct {
		ID    string `json:"id"`
		Usage *struct {
			InputTokens       int64 `json:"input_tokens"`
			OutputTokens      int64 `json:"output_tokens"`
			CacheCreateTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadTokens   int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type request struct {
	day    string
	counts Counts
}

// Scan reads every *.jsonl file under root (subagent logs included) and
// returns token counts per day in loc. Each request is counted once; when its
// lines disagree, the largest value of each field wins.
func Scan(root string, loc *time.Location) (map[string]Counts, error) {
	reqs := map[string]request{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return scanLog(f, loc, reqs)
	})
	if err != nil {
		return nil, err
	}

	days := map[string]Counts{}
	for _, r := range reqs {
		c := days[r.day]
		c.add(r.counts)
		days[r.day] = c
	}
	return days, nil
}

func scanLog(r io.Reader, loc *time.Location, reqs map[string]request) error {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 && bytes.Contains(line, []byte(`"usage"`)) {
			addLine(line, loc, reqs)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func addLine(line []byte, loc *time.Location, reqs map[string]request) {
	var l logLine
	if json.Unmarshal(line, &l) != nil || l.Type != "assistant" || l.Message.Usage == nil || l.Timestamp.IsZero() {
		return
	}
	key := l.RequestID
	if key == "" {
		key = l.Message.ID
	}
	if key == "" {
		return
	}
	u := l.Message.Usage
	c := Counts{Input: u.InputTokens, Output: u.OutputTokens, CacheWrite: u.CacheCreateTokens, CacheRead: u.CacheReadTokens}
	r, seen := reqs[key]
	if !seen {
		reqs[key] = request{day: l.Timestamp.In(loc).Format(time.DateOnly), counts: c}
		return
	}
	r.counts.Input = max(r.counts.Input, c.Input)
	r.counts.Output = max(r.counts.Output, c.Output)
	r.counts.CacheWrite = max(r.counts.CacheWrite, c.CacheWrite)
	r.counts.CacheRead = max(r.counts.CacheRead, c.CacheRead)
	reqs[key] = r
}
