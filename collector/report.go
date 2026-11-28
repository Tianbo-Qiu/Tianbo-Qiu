package main

import (
	"sort"
	"time"

	"github.com/Tianbo-Qiu/Tianbo-Qiu/collector/tokenlog"
)

// DayStat is one day's activity across all tracked repos.
type DayStat struct {
	Date     string // YYYY-MM-DD
	Add, Del int
	Commits  int
	Files    int // distinct counted files touched
}

// LangRow is a line of the languages table. The last row may be "Other",
// with Others naming the languages folded into it.
type LangRow struct {
	Name   string
	Lines  int
	Frac   float64
	Others []string
}

// Report is everything the widget shows.
type Report struct {
	Now   time.Time
	Owner string // GitHub account, for the footer

	Shown       DayStat          // today, or the last day with commits
	ShownToday  bool             //
	ShownTokens *tokenlog.Counts // tokens recorded for the shown day, if any

	LOC         int
	LOCDelta    int  // vs. the previous snapshot
	HasBaseline bool // false on the very first run: no delta to show
	Repos       int
	Commits     int // all time

	Days  []DayStat // the last 30 days, oldest first, ending today
	Langs []LangRow

	Tokens      *tokenlog.Counts // all recorded days; nil without stats/tokens.json
	TokensSince string           // first recorded day
}

const (
	windowDays = 30
	topLangs   = 3
)

func buildReport(snap *Snapshot, history map[string][]commit, tokens *tokenlog.File, now time.Time, loc *time.Location, owner string) *Report {
	r := &Report{Now: now, Owner: owner, Repos: len(snap.Repos), HasBaseline: snap.Baseline != ""}

	type acc struct {
		DayStat
		files map[string]bool
	}
	byDay := map[string]*acc{}
	for repo, commits := range history {
		for _, c := range commits {
			if c.at.After(now) {
				continue
			}
			r.Commits++
			date := c.at.In(loc).Format(time.DateOnly)
			a := byDay[date]
			if a == nil {
				a = &acc{DayStat: DayStat{Date: date}, files: map[string]bool{}}
				byDay[date] = a
			}
			a.Add += c.add
			a.Del += c.del
			a.Commits++
			for _, p := range c.paths {
				a.files[repo+"/"+p] = true
			}
		}
	}
	stat := func(date string) DayStat {
		a := byDay[date]
		if a == nil {
			return DayStat{Date: date}
		}
		s := a.DayStat
		s.Files = len(a.files)
		return s
	}

	today := now.Format(time.DateOnly)
	for i := windowDays - 1; i >= 0; i-- {
		d := time.Date(now.Year(), now.Month(), now.Day()-i, 0, 0, 0, 0, loc)
		r.Days = append(r.Days, stat(d.Format(time.DateOnly)))
	}

	// Show today once it has commits; until then, the last day that did.
	r.Shown, r.ShownToday = stat(today), true
	if r.Shown.Commits == 0 {
		latest := ""
		for date, a := range byDay {
			if date < today && a.Commits > 0 && date > latest {
				latest = date
			}
		}
		if latest != "" {
			r.Shown, r.ShownToday = stat(latest), false
		}
	}

	langs := map[string]int{}
	for _, repo := range snap.Repos {
		r.LOC += repo.LOC
		r.LOCDelta += repo.LOCDelta
		for l, n := range repo.Languages {
			langs[l] += n
		}
	}
	r.Langs = langRows(langs)

	if tokens != nil && len(tokens.Days) > 0 {
		t := tokens.Total()
		r.Tokens, r.TokensSince = &t, tokens.Since()
		if c, ok := tokens.Days[r.Shown.Date]; ok {
			r.ShownTokens = &c
		}
	}
	return r
}

// langRows returns the largest languages by line count, folding the rest
// into "Other".
func langRows(langs map[string]int) []LangRow {
	total := 0
	names := make([]string, 0, len(langs))
	for l, n := range langs {
		total += n
		names = append(names, l)
	}
	if total == 0 {
		return nil
	}
	sort.Slice(names, func(i, j int) bool {
		if langs[names[i]] != langs[names[j]] {
			return langs[names[i]] > langs[names[j]]
		}
		return names[i] < names[j]
	})

	var rows []LangRow
	for i, l := range names {
		if i == topLangs && len(names) > topLangs+1 {
			other := LangRow{Name: "Other", Others: names[i:]}
			for _, o := range names[i:] {
				other.Lines += langs[o]
			}
			other.Frac = float64(other.Lines) / float64(total)
			return append(rows, other)
		}
		rows = append(rows, LangRow{Name: l, Lines: langs[l], Frac: float64(langs[l]) / float64(total)})
	}
	return rows
}
