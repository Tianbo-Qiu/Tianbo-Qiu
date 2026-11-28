package main

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Tianbo-Qiu/Tianbo-Qiu/collector/tokenlog"
)

func pacific(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func testSnapshot() *Snapshot {
	return &Snapshot{
		Date:     "2026-11-27",
		Baseline: "2026-11-26",
		Repos: []RepoStats{
			{Name: "a", LOC: 1000, LOCDelta: 40, Languages: map[string]int{"Go": 700, "Markdown": 200, "YAML": 60, "SQL": 40}},
			{Name: "b", LOC: 500, Languages: map[string]int{"JavaScript": 500}},
		},
	}
}

func TestBuildReportShowsLastActiveDayWhenTodayIsQuiet(t *testing.T) {
	loc := pacific(t)
	now := time.Date(2026, 11, 27, 9, 0, 0, 0, loc)
	at := func(day, hour int) time.Time { return time.Date(2026, 11, day, hour, 0, 0, 0, loc) }
	history := map[string][]commit{
		"a": {
			{at: at(25, 10), add: 10, del: 2, paths: []string{"main.go", "x.go"}},
			{at: at(25, 23), add: 5, paths: []string{"main.go"}},
			{at: at(20, 12), add: 99, paths: []string{"old.go"}},
		},
		"b": {
			{at: at(25, 11), add: 1, del: 1, paths: []string{"main.go"}}, // same name, other repo
			{at: at(28, 1), add: 1000, paths: []string{"future.go"}},     // after now: ignored
		},
	}
	tok := &tokenlog.File{Days: map[string]tokenlog.Counts{
		"2026-11-25": {Output: 7, CacheRead: 93},
		"2026-11-01": {Output: 3},
	}}

	r := buildReport(testSnapshot(), history, tok, now, loc, "me")

	want := DayStat{Date: "2026-11-25", Add: 16, Del: 3, Commits: 3, Files: 3}
	if r.Shown != want || r.ShownToday {
		t.Errorf("Shown = %+v (today=%v), want %+v", r.Shown, r.ShownToday, want)
	}
	if r.ShownTokens == nil || r.ShownTokens.Total() != 100 {
		t.Errorf("ShownTokens = %+v", r.ShownTokens)
	}
	if r.Tokens.Total() != 103 || r.TokensSince != "2026-11-01" {
		t.Errorf("Tokens = %+v since %s", r.Tokens, r.TokensSince)
	}
	if r.Commits != 4 || r.LOC != 1500 || r.LOCDelta != 40 || r.Repos != 2 || !r.HasBaseline {
		t.Errorf("totals: commits %d loc %d delta %d repos %d", r.Commits, r.LOC, r.LOCDelta, r.Repos)
	}
	if len(r.Days) != 30 || r.Days[0].Date != "2026-10-29" || r.Days[29].Date != "2026-11-27" {
		t.Fatalf("Days span %s..%s (%d)", r.Days[0].Date, r.Days[len(r.Days)-1].Date, len(r.Days))
	}
	if d := r.Days[27]; d.Date != "2026-11-25" || d.Add != 16 {
		t.Errorf("Days[27] = %+v", d)
	}
}

func TestBuildReportPrefersTodayOnceItHasCommits(t *testing.T) {
	loc := pacific(t)
	now := time.Date(2026, 11, 27, 16, 0, 0, 0, loc)
	history := map[string][]commit{"a": {
		{at: time.Date(2026, 11, 27, 0, 30, 0, 0, loc), add: 3, paths: []string{"a.go"}},
		{at: time.Date(2026, 11, 26, 12, 0, 0, 0, loc), add: 50, paths: []string{"b.go"}},
	}}
	r := buildReport(testSnapshot(), history, nil, now, loc, "me")
	if !r.ShownToday || r.Shown.Add != 3 || r.Tokens != nil || r.ShownTokens != nil {
		t.Errorf("Shown = %+v today=%v tokens=%v", r.Shown, r.ShownToday, r.Tokens)
	}
}

func TestLangRowsFoldsTail(t *testing.T) {
	rows := langRows(map[string]int{"Go": 700, "JavaScript": 500, "Markdown": 200, "YAML": 60, "SQL": 40})
	if len(rows) != 4 || rows[0].Name != "Go" || rows[3].Name != "Other" || rows[3].Lines != 100 ||
		strings.Join(rows[3].Others, ",") != "YAML,SQL" {
		t.Errorf("rows = %+v", rows)
	}
	// Exactly one language past the top three is shown by name, not as "Other".
	rows = langRows(map[string]int{"Go": 4, "JavaScript": 3, "Markdown": 2, "YAML": 1})
	if rows[3].Name != "YAML" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestRenderIsWellFormedXML(t *testing.T) {
	loc := pacific(t)
	now := time.Date(2026, 11, 27, 16, 5, 0, 0, loc)
	history := map[string][]commit{"a": {{at: now.Add(-time.Hour), add: 346, paths: []string{"a.go"}}}}
	tok := &tokenlog.File{Days: map[string]tokenlog.Counts{"2026-11-27": {Input: 1, Output: 2, CacheWrite: 3, CacheRead: 400}}}
	for _, r := range []*Report{
		buildReport(testSnapshot(), history, tok, now, loc, "me"),
		buildReport(&Snapshot{Date: "2026-11-27"}, nil, nil, now, loc, "me"), // no data at all
	} {
		svg := render(r)
		dec := xml.NewDecoder(strings.NewReader(svg))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("render produced bad XML: %v", err)
			}
		}
	}
}
