package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestLinesAt builds a repo with one commit yesterday and one today and checks
// that lines of code are counted as the branch stood at each moment.
func TestLinesAt(t *testing.T) {
	dir := t.TempDir()
	loc := pacific(t)
	midnight := time.Date(2026, 11, 27, 0, 0, 0, 0, loc)

	git := func(when time.Time, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		date := when.Format(time.RFC3339)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_AUTHOR_DATE="+date,
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_COMMITTER_DATE="+date,
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git(midnight, "init", "-q", "-b", "main")
	write("main.go", "package main\n\nfunc main() {}\n")                         // 2 non-blank lines
	write("go.sum", "x v1 h1:abc\n")                                             // excluded
	write("notes.bin", "a\x00b\n")                                               // unknown extension
	if err := os.Symlink("main.go", filepath.Join(dir, "link.go")); err != nil { // symlink: skipped
		t.Fatal(err)
	}
	git(midnight.Add(-2*time.Hour), "add", ".")
	git(midnight.Add(-2*time.Hour), "commit", "-q", "-m", "yesterday")

	write("main.go", "package main\n\nfunc main() {\n\tprintln(1)\n}\n") // 4
	write("README.md", "# hi\n")                                         // 1
	git(midnight.Add(3*time.Hour), "add", ".")
	git(midnight.Add(3*time.Hour), "commit", "-q", "-m", "today")

	for _, tc := range []struct {
		at   time.Time
		want map[string]int
	}{
		{midnight.Add(-3 * time.Hour), map[string]int{}},                      // before the first commit
		{midnight, map[string]int{"Go": 2}},                                   // start of today
		{midnight.Add(5 * time.Hour), map[string]int{"Go": 4, "Markdown": 1}}, // now
	} {
		got, err := linesAt(dir, tc.at)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(tc.want) {
			t.Errorf("at %s: got %v, want %v", tc.at, got, tc.want)
			continue
		}
		for l, n := range tc.want {
			if got[l] != n {
				t.Errorf("at %s: got %v, want %v", tc.at, got, tc.want)
			}
		}
	}
}
