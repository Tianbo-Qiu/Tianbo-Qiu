package tokenlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanCountsEachRequestOnceByDay(t *testing.T) {
	root := t.TempDir()
	session := `{"type":"user","timestamp":"2026-11-27T20:00:00Z","message":{"content":"mentions \"usage\" but is not a response"}}
{"type":"assistant","timestamp":"2026-11-27T20:00:01Z","requestId":"req_a","message":{"id":"msg_a","usage":{"input_tokens":3,"output_tokens":10,"cache_creation_input_tokens":100,"cache_read_input_tokens":1000}}}
{"type":"assistant","timestamp":"2026-11-27T20:00:02Z","requestId":"req_a","message":{"id":"msg_a","usage":{"input_tokens":3,"output_tokens":25,"cache_creation_input_tokens":100,"cache_read_input_tokens":1000}}}
{"type":"assistant","timestamp":"2026-11-28T07:30:00Z","requestId":"req_b","message":{"id":"msg_b","usage":{"input_tokens":1,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":50}}}
not json
`
	// A resumed session copies earlier lines into a new file; they must not count twice.
	copied := `{"type":"assistant","timestamp":"2026-11-27T20:00:02Z","requestId":"req_a","message":{"id":"msg_a","usage":{"input_tokens":3,"output_tokens":25,"cache_creation_input_tokens":100,"cache_read_input_tokens":1000}}}
`
	write(t, filepath.Join(root, "proj", "s1.jsonl"), session)
	write(t, filepath.Join(root, "proj", "s1", "subagents", "a.jsonl"), copied)
	write(t, filepath.Join(root, "proj", "notes.txt"), `{"type":"assistant","usage":{}}`)

	la, _ := time.LoadLocation("America/Los_Angeles")
	days, err := Scan(root, la)
	if err != nil {
		t.Fatal(err)
	}
	// req_a counts once with its largest values; req_b (07:30 UTC on the 28th)
	// is still the 27th in Pacific time.
	want := map[string]Counts{
		"2026-11-27": {Input: 3 + 1, Output: 25 + 5, CacheWrite: 100, CacheRead: 1000 + 50},
	}
	if len(days) != len(want) {
		t.Fatalf("days = %v, want %v", days, want)
	}
	for d, c := range want {
		if days[d] != c {
			t.Errorf("%s = %+v, want %+v", d, days[d], c)
		}
	}
}

func TestMergeKeepsLargerTotals(t *testing.T) {
	f := &File{Days: map[string]Counts{
		"2026-10-01": {Output: 500, CacheRead: 5000}, // logs since deleted
		"2026-11-26": {Output: 10},
	}}
	changed := f.Merge(map[string]Counts{
		"2026-10-01": {Output: 20},   // partial: some logs gone
		"2026-11-26": {Output: 40},   // grew
		"2026-11-27": {CacheRead: 9}, // new day
	})
	if !changed {
		t.Fatal("Merge reported no change")
	}
	if got := f.Days["2026-10-01"]; got.Output != 500 {
		t.Errorf("deleted-log day was overwritten: %+v", got)
	}
	if got := f.Days["2026-11-26"]; got.Output != 40 {
		t.Errorf("grown day not updated: %+v", got)
	}
	if got := f.Total(); got.Total() != 500+5000+40+9 {
		t.Errorf("Total = %+v", got)
	}
	if f.Since() != "2026-10-01" {
		t.Errorf("Since = %q", f.Since())
	}
	if f.Merge(map[string]Counts{"2026-11-26": {Output: 40}}) {
		t.Error("identical counts reported as a change")
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	f := &File{UpdatedAt: time.Date(2026, 11, 27, 23, 30, 0, 0, time.UTC), Days: map[string]Counts{"2026-11-27": {Input: 1, Output: 2, CacheWrite: 3, CacheRead: 4}}}
	data, err := f.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	g, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if g.Days["2026-11-27"] != f.Days["2026-11-27"] || !g.UpdatedAt.Equal(f.UpdatedAt) {
		t.Errorf("round trip: %+v", g)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
