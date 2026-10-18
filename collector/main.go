// Command collector computes daily code activity for the repos in repos.txt,
// writes a snapshot to stats/YYYY-MM-DD.json and renders the profile widget
// to assets/code-stats-{dark,light}.svg.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata" // day boundaries must not depend on the runner's zoneinfo
)

func main() {
	root := flag.String("root", "..", "profile repo root (holds collector/, stats/, assets/)")
	tzName := flag.String("tz", "America/Los_Angeles", "timezone that defines a day")
	nowFlag := flag.String("now", "", "override the current time (RFC3339), for testing")
	flag.Parse()

	loc, err := time.LoadLocation(*tzName)
	if err != nil {
		log.Fatal(err)
	}
	now := time.Now().In(loc)
	if *nowFlag != "" {
		t, err := time.Parse(time.RFC3339, *nowFlag)
		if err != nil {
			log.Fatalf("-now: %v", err)
		}
		now = t.In(loc)
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	today := dayStart.Format(time.DateOnly)

	repos, err := readRepoList(filepath.Join(*root, "collector", "repos.txt"))
	if err != nil {
		log.Fatal(err)
	}

	work, err := os.MkdirTemp("", "code-stats-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(work)

	statsDir := filepath.Join(*root, "stats")
	prev, err := loadPrevious(statsDir, today)
	if err != nil {
		log.Fatal(err)
	}

	snap := &Snapshot{Date: today, UpdatedAt: now}
	if prev != nil {
		snap.Baseline = prev.Date
	}
	history := map[string][]commit{}
	for _, repo := range repos {
		log.Printf("collecting %s", repo)
		dir, err := cloneRepo(work, repo)
		if err != nil {
			log.Fatal(err)
		}
		commits, err := readCommits(dir)
		if err != nil {
			log.Fatal(err)
		}
		langs, err := countLines(dir)
		if err != nil {
			log.Fatal(err)
		}

		name := repoName(repo)
		history[name] = commits
		r := RepoStats{Name: name, Languages: langs}
		for _, c := range commits {
			if !c.at.Before(dayStart) && !c.at.After(now) {
				r.Commits++
				r.Additions += c.add
				r.Deletions += c.del
			}
		}
		for _, n := range langs {
			r.LOC += n
		}
		r.LOCDelta = locDelta(prev, name, r.LOC)
		snap.Repos = append(snap.Repos, r)
	}
	snap.LastActive = lastActivity(history, dayStart, loc)

	if err := saveSnapshot(statsDir, snap); err != nil {
		log.Fatal(err)
	}

	assets := filepath.Join(*root, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		log.Fatal(err)
	}
	for name, t := range map[string]theme{"dark": darkTheme, "light": lightTheme} {
		path := filepath.Join(assets, fmt.Sprintf("code-stats-%s.svg", name))
		if err := os.WriteFile(path, []byte(render(snap, now, t)), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("wrote snapshot %s and widgets", today)
}

// readRepoList reads owner/name lines, skipping blanks and # comments.
func readRepoList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var repos []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Count(line, "/") != 1 {
			return nil, fmt.Errorf("%s: %q is not owner/name", path, line)
		}
		repos = append(repos, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("%s: no repos listed", path)
	}
	return repos, nil
}

func repoName(repo string) string {
	_, name, _ := strings.Cut(repo, "/")
	return name
}

// lastActivity finds the most recent day before dayStart with commits and
// reports the busiest repo on that day, for the quiet-day line.
func lastActivity(history map[string][]commit, dayStart time.Time, loc *time.Location) *DayActivity {
	var latest time.Time
	for _, commits := range history {
		for _, c := range commits {
			if c.at.Before(dayStart) && c.at.After(latest) {
				latest = c.at
			}
		}
	}
	if latest.IsZero() {
		return nil
	}

	l := latest.In(loc)
	start := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	var best *DayActivity
	for name, commits := range history {
		a := DayActivity{Date: start.Format(time.DateOnly), Repo: name}
		for _, c := range commits {
			if !c.at.Before(start) && c.at.Before(end) {
				a.Additions += c.add
				a.Deletions += c.del
			}
		}
		total := a.Additions + a.Deletions
		if best == nil || total > best.Additions+best.Deletions ||
			(total == best.Additions+best.Deletions && name < best.Repo) {
			best = &a
		}
	}
	return best
}
