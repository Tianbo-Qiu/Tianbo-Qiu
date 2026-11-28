// Command report-tokens totals Claude Code token usage per day from the local
// session logs and records it in stats/tokens.json on GitHub, where the
// profile widget picks it up. It talks to GitHub through the gh CLI, so it
// needs no checkout of the profile repo, only `gh auth login`.
//
// Only daily totals by token type leave the machine: no project names, paths
// or prompts. Run it daily; see README.md for the launchd job.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/Tianbo-Qiu/Tianbo-Qiu/collector/tokenlog"
)

func main() {
	home, _ := os.UserHomeDir()
	projects := flag.String("projects", filepath.Join(home, ".claude", "projects"), "Claude Code session logs")
	repo := flag.String("repo", "Tianbo-Qiu/Tianbo-Qiu", "profile repo (owner/name)")
	path := flag.String("path", "stats/tokens.json", "file in the repo")
	tzName := flag.String("tz", "America/Los_Angeles", "timezone that defines a day; must match the collector's")
	dryRun := flag.Bool("dry-run", false, "print the merged file instead of uploading it")
	flag.Parse()

	loc, err := time.LoadLocation(*tzName)
	if err != nil {
		log.Fatal(err)
	}
	fresh, err := tokenlog.Scan(*projects, loc)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("scanned %d days of logs in %s", len(fresh), *projects)

	f, sha, err := fetch(*repo, *path)
	if err != nil {
		log.Fatal(err)
	}
	if !f.Merge(fresh) && sha != "" {
		log.Print("tokens.json is up to date")
		return
	}
	f.UpdatedAt = time.Now().In(loc).Truncate(time.Second)
	data, err := f.Marshal()
	if err != nil {
		log.Fatal(err)
	}
	if *dryRun {
		os.Stdout.Write(data)
		return
	}
	if err := upload(*repo, *path, sha, data); err != nil {
		log.Fatal(err)
	}
	log.Printf("updated %s in %s (%d days)", *path, *repo, len(f.Days))
}

// fetch returns the current file and its blob sha, or an empty file and ""
// if it doesn't exist yet.
func fetch(repo, path string) (*tokenlog.File, string, error) {
	out, err := gh(nil, "api", fmt.Sprintf("repos/%s/contents/%s", repo, path))
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return &tokenlog.File{Days: map[string]tokenlog.Counts{}}, "", nil
		}
		return nil, "", err
	}
	var resp struct {
		SHA     string `json:"sha"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, "", err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(resp.Content, "\n", ""))
	if err != nil {
		return nil, "", err
	}
	f, err := tokenlog.Parse(raw)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	return f, resp.SHA, nil
}

// upload writes the file with one commit. The sha makes GitHub reject the
// write if the file changed since fetch; the next run then merges again.
func upload(repo, path, sha string, data []byte) error {
	body := map[string]string{
		"message": "chore: update token stats",
		"content": base64.StdEncoding.EncodeToString(data),
	}
	if sha != "" {
		body["sha"] = sha
	}
	in, err := json.Marshal(body)
	if err != nil {
		return err
	}
	_, err = gh(in, "api", "-X", "PUT", fmt.Sprintf("repos/%s/contents/%s", repo, path), "--input", "-")
	return err
}

func gh(stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("gh %s: %s", args[1], strings.TrimSpace(stderr.String()))
		}
		return nil, err
	}
	return out, nil
}
