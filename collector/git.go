package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type commit struct {
	at       time.Time // author date
	add, del int       // counted lines only (see excluded/languageOf)
}

func cloneRepo(workDir, repo string) (string, error) {
	dir := filepath.Join(workDir, strings.ReplaceAll(repo, "/", "__"))
	if _, err := runGit("", "clone", "--quiet", "https://github.com/"+repo+".git", dir); err != nil {
		return "", err
	}
	return dir, nil
}

// readCommits returns every non-merge, non-bot commit on the default branch,
// with additions/deletions limited to files we count as code. Other branches
// are skipped: they hold unmerged work or build output (e.g. gh-pages).
func readCommits(dir string) ([]commit, error) {
	out, err := runGit(dir, "log", "HEAD", "--no-merges",
		"--numstat", "--format=%x00%at%x09%an")
	if err != nil {
		return nil, err
	}

	var commits []commit
	cur := -1
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "\x00"); ok {
			cur = -1
			at, author, _ := strings.Cut(rest, "\t")
			if strings.HasSuffix(author, "[bot]") {
				continue
			}
			sec, err := strconv.ParseInt(at, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse author date %q: %w", at, err)
			}
			commits = append(commits, commit{at: time.Unix(sec, 0)})
			cur = len(commits) - 1
			continue
		}
		if cur < 0 {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		add, errA := strconv.Atoi(parts[0])
		del, errD := strconv.Atoi(parts[1])
		if errA != nil || errD != nil { // "-" marks a binary file
			continue
		}
		path := numstatPath(parts[2])
		if excluded(path) || languageOf(path) == "" {
			continue
		}
		commits[cur].add += add
		commits[cur].del += del
	}
	return commits, nil
}

// numstatPath resolves rename notation ("a/{old => new}.go", "old => new")
// to the new path.
func numstatPath(p string) string {
	open, close := strings.Index(p, "{"), strings.Index(p, "}")
	if open >= 0 && close > open {
		if _, newPart, ok := strings.Cut(p[open+1:close], " => "); ok {
			return strings.ReplaceAll(p[:open]+newPart+p[close+1:], "//", "/")
		}
	}
	if _, newPath, ok := strings.Cut(p, " => "); ok {
		return newPath
	}
	return p
}

// countLines counts non-blank lines per language over tracked files.
func countLines(dir string) (map[string]int, error) {
	out, err := runGit(dir, "ls-files", "-z")
	if err != nil {
		return nil, err
	}

	langs := map[string]int{}
	for _, p := range strings.Split(out, "\x00") {
		if p == "" || excluded(p) {
			continue
		}
		lang := languageOf(p)
		if lang == "" {
			continue
		}
		full := filepath.Join(dir, p)
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		if bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		n := 0
		for _, l := range bytes.Split(data, []byte("\n")) {
			if len(bytes.TrimSpace(l)) > 0 {
				n++
			}
		}
		if n > 0 {
			langs[lang] += n
		}
	}
	return langs, nil
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}
