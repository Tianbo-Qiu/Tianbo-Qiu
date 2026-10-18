package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Snapshot is one day's stats. It is rewritten on every run during the day,
// so the file for a past day holds that day's final numbers.
type Snapshot struct {
	Date       string       `json:"date"`
	UpdatedAt  time.Time    `json:"updated_at"`
	Baseline   string       `json:"baseline,omitempty"` // date LOC deltas compare against; empty on the first day
	Repos      []RepoStats  `json:"repos"`
	LastActive *DayActivity `json:"last_active,omitempty"`
}

type RepoStats struct {
	Name      string         `json:"name"`
	Commits   int            `json:"commits"`
	Additions int            `json:"additions"`
	Deletions int            `json:"deletions"`
	LOC       int            `json:"loc"`
	LOCDelta  int            `json:"loc_delta"` // vs. the previous day's snapshot
	Languages map[string]int `json:"languages"`
}

// DayActivity is the busiest repo on the most recent day with commits.
type DayActivity struct {
	Date      string `json:"date"`
	Repo      string `json:"repo"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// loadPrevious returns the latest snapshot dated before today, or nil.
func loadPrevious(dir, today string) (*Snapshot, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	best := ""
	for _, e := range entries {
		date, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || len(date) != len(time.DateOnly) {
			continue
		}
		if date < today && date > best {
			best = date
		}
	}
	if best == "" {
		return nil, nil
	}

	data, err := os.ReadFile(filepath.Join(dir, best+".json"))
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// locDelta compares against the previous snapshot. With no history at all
// the delta is 0; a repo that's new since then counts in full.
func locDelta(prev *Snapshot, name string, loc int) int {
	if prev == nil {
		return 0
	}
	for _, r := range prev.Repos {
		if r.Name == name {
			return loc - r.LOC
		}
	}
	return loc
}

func saveSnapshot(dir string, s *Snapshot) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.Date+".json"), append(data, '\n'), 0o644)
}
