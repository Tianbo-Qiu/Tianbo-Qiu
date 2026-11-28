// Command collector reads code activity from the repos listed in
// $CODE_STATS_REPOS and renders the profile widget to assets/code-report.svg,
// with token totals from stats/tokens.json. It writes nothing else: every
// number comes from git.
//
// The repo list lives outside this public repo (an Actions variable in CI) and
// is never logged, so tracking a private repo doesn't reveal its name.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata" // day boundaries must not depend on the runner's zoneinfo
	"unicode"

	"github.com/Tianbo-Qiu/Tianbo-Qiu/collector/tokenlog"
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

	repos, err := parseRepoList(os.Getenv(reposEnv))
	if err != nil {
		log.Fatal(err)
	}
	maskInActionsLogs(repos)

	work, err := os.MkdirTemp("", "code-stats-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(work)

	data := map[string]*repoData{}
	for i, repo := range repos {
		log.Printf("collecting repo %d of %d", i+1, len(repos))
		d, err := collect(work, repo, dayStart, now)
		if err != nil {
			log.Fatalf("repo %d: %v", i+1, err)
		}
		data[repo] = d
	}

	tokens, err := tokenlog.Load(filepath.Join(*root, "stats", "tokens.json"))
	if err != nil {
		log.Fatal(err)
	}
	owner, _, _ := strings.Cut(repos[0], "/")
	report := buildReport(data, tokens, now, loc, owner)

	assets := filepath.Join(*root, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "code-report.svg"), []byte(render(report)), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote assets/code-report.svg")
}

// collect clones one repo and reads its commits and its lines of code now and
// at the start of today.
func collect(work, repo string, dayStart, now time.Time) (*repoData, error) {
	dir, err := cloneRepo(work, repo)
	if err != nil {
		return nil, err
	}
	d := &repoData{}
	if d.commits, err = readCommits(dir); err != nil {
		return nil, err
	}
	if d.langs, err = linesAt(dir, now); err != nil {
		return nil, err
	}
	if d.before, err = linesAt(dir, dayStart); err != nil {
		return nil, err
	}
	return d, nil
}

// linesAt counts lines of code per language as the default branch stood at t.
func linesAt(dir string, t time.Time) (map[string]int, error) {
	rev, err := revAt(dir, t)
	if err != nil {
		return nil, err
	}
	return countLines(dir, rev)
}

// reposEnv names the variable holding the repos to count: owner/name entries
// separated by spaces, commas or newlines.
const reposEnv = "CODE_STATS_REPOS"

func parseRepoList(s string) ([]string, error) {
	var repos []string
	seen := map[string]bool{}
	for _, r := range strings.FieldsFunc(s, func(c rune) bool { return c == ',' || unicode.IsSpace(c) }) {
		if strings.Count(r, "/") != 1 || strings.HasPrefix(r, "/") || strings.HasSuffix(r, "/") {
			return nil, fmt.Errorf("%s: entry %d is not owner/name", reposEnv, len(repos)+1)
		}
		if !seen[r] {
			seen[r] = true
			repos = append(repos, r)
		}
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("%s is empty: set it to the repos to count, e.g. owner/a,owner/b", reposEnv)
	}
	return repos, nil
}

// maskInActionsLogs tells GitHub Actions to hide each repo's full and short
// name wherever it would appear in the log, including git's error messages.
func maskInActionsLogs(repos []string) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return
	}
	for _, r := range repos {
		_, name, _ := strings.Cut(r, "/")
		fmt.Printf("::add-mask::%s\n::add-mask::%s\n", r, name)
	}
}
