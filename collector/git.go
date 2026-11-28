package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
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
	paths    []string  // counted files the commit touched
}

// tokenEnv names the variable holding a GitHub token for private repos.
const tokenEnv = "CODE_STATS_TOKEN"

// cloneRepo clones github.com/<repo>. When CODE_STATS_TOKEN is set the token
// is sent as an auth header through git's environment config, so it never
// appears in the command line or in error messages. If GitHub rejects the
// token (expired, revoked), the clone is retried without it, so public repos
// keep being counted and only private ones fail.
func cloneRepo(workDir, repo string) (string, error) {
	dir := filepath.Join(workDir, strings.ReplaceAll(repo, "/", "__"))
	url := "https://github.com/" + repo + ".git"
	noPrompt := []string{"GIT_TERMINAL_PROMPT=0"} // fail instead of asking for a password

	tok := os.Getenv(tokenEnv)
	if tok == "" {
		_, err := runGitEnv("", noPrompt, "clone", "--quiet", url, dir)
		return dir, err
	}
	auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + tok))
	withToken := append(noPrompt,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic "+auth,
	)
	_, tokErr := runGitEnv("", withToken, "clone", "--quiet", url, dir)
	if tokErr == nil {
		return dir, nil
	}
	os.RemoveAll(dir)
	if _, err := runGitEnv("", noPrompt, "clone", "--quiet", url, dir); err != nil {
		return "", tokErr
	}
	log.Printf("warning: %s was rejected; cloned without it (public repo)", tokenEnv)
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
		commits[cur].paths = append(commits[cur].paths, path)
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

// revAt returns the default branch's commit as of t (the last commit on its
// first-parent line committed at or before t), or "" if there was none yet.
func revAt(dir string, t time.Time) (string, error) {
	out, err := runGit(dir, "rev-list", "-1", "--first-parent", "--before="+t.Format(time.RFC3339), "HEAD")
	return strings.TrimSpace(out), err
}

// countLines counts non-blank lines per language in the files tracked at rev,
// read from git's object store so any past commit can be counted. An empty
// rev counts nothing.
func countLines(dir, rev string) (map[string]int, error) {
	langs := map[string]int{}
	if rev == "" {
		return langs, nil
	}
	out, err := runGit(dir, "ls-tree", "-r", "-z", rev)
	if err != nil {
		return nil, err
	}

	// Each entry is "<mode> <type> <object>\t<path>". Only regular files count:
	// symlinks (120000) and submodules (160000) are skipped.
	var objects, fileLangs []string
	for _, entry := range strings.Split(out, "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 3 || (f[0] != "100644" && f[0] != "100755") || excluded(path) {
			continue
		}
		if lang := languageOf(path); lang != "" {
			objects = append(objects, f[2])
			fileLangs = append(fileLangs, lang)
		}
	}
	if len(objects) == 0 {
		return langs, nil
	}

	cmd := exec.Command("git", "cat-file", "--batch")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(strings.Join(objects, "\n") + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	r := bufio.NewReader(stdout)
	for _, lang := range fileLangs {
		// "<object> blob <size>\n<content>\n"
		header, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("git cat-file: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		f := strings.Fields(header)
		if len(f) != 3 || f[1] != "blob" {
			return nil, fmt.Errorf("git cat-file: unexpected header %q", header)
		}
		size, err := strconv.Atoi(f[2])
		if err != nil {
			return nil, fmt.Errorf("git cat-file: bad size in %q", header)
		}
		data := make([]byte, size+1) // content plus the trailing newline
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, fmt.Errorf("git cat-file: %w", err)
		}
		data = data[:size]
		if bytes.IndexByte(data, 0) >= 0 { // binary
			continue
		}
		for _, l := range bytes.Split(data, []byte("\n")) {
			if len(bytes.TrimSpace(l)) > 0 {
				langs[lang]++
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("git cat-file: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return langs, nil
}

func runGit(dir string, args ...string) (string, error) {
	return runGitEnv(dir, nil, args...)
}

// runGitEnv runs git with extra environment variables on top of ours.
func runGitEnv(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}
