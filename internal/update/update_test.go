package update

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestParseCompare(t *testing.T) {
	changes, ok, err := parseCompare([]byte(`{"status":"ahead","commits":[
		{"commit":{"message":"Older\n\nWhy it changed.\n\nCo-Authored-By: Someone <x@y>","author":{"date":"2026-11-01T10:00:00Z"}}},
		{"commit":{"message":"Newer","author":{"date":"2026-11-02T10:00:00Z"}}},
		{"commit":{"message":"Merge pull request #3"},"parents":[{},{}]}]}`))
	if err != nil || !ok || len(changes) != 2 || changes[0].Subject != "Newer" || changes[1].Subject != "Older" {
		t.Fatalf("ahead: got %+v %v %v, want the commits newest first", changes, ok, err)
	}
	if changes[1].Body != "Why it changed." {
		t.Fatalf("body: got %q, want it without its trailer", changes[1].Body)
	}
	if changes, ok, _ := parseCompare([]byte(`{"status":"identical","commits":[]}`)); !ok || len(changes) != 0 {
		t.Fatalf("identical: got %+v %v, want up to date", changes, ok)
	}
	for _, status := range []string{"behind", "diverged"} {
		if _, ok, _ := parseCompare([]byte(`{"status":"` + status + `","commits":[{"commit":{"message":"x"}}]}`)); ok {
			t.Fatalf("%s: a build main does not contain should never be offered an update", status)
		}
	}
}

func TestCheckCachesAndDismiss(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cache", "update.json")
	calls := 0
	compare := func(context.Context, string) ([]Change, bool, error) {
		calls++
		return []Change{{Subject: "New thing"}}, true, nil
	}
	now := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	st, err := Check(context.Background(), file, "abc", now, Fresh, compare)
	if err != nil || st.Behind != 1 || !st.Offer(now) {
		t.Fatalf("got %+v %v, want 1 commit behind, offered", st, err)
	}
	if _, err := Check(context.Background(), file, "abc", now.Add(10*time.Minute), Fresh, compare); err != nil || calls != 1 {
		t.Fatalf("a fresh check should be reused, got %d calls", calls)
	}
	if _, err := Check(context.Background(), file, "abc", now.Add(10*time.Minute), 0, compare); err != nil || calls != 2 {
		t.Fatalf("no max age should always ask main, got %d calls", calls)
	}
	if _, err := Check(context.Background(), file, "def", now.Add(10*time.Minute), Fresh, compare); err != nil || calls != 3 {
		t.Fatalf("a new build should be checked again, got %d calls", calls)
	}
	if err := Dismiss(file, now); err != nil {
		t.Fatal(err)
	}
	st, _ = Check(context.Background(), file, "def", now.Add(5*time.Hour), Fresh, compare)
	if st.Offer(now.Add(5 * time.Hour)) {
		t.Fatal("an update put off should not be offered again the same day")
	}
	if !st.Offer(now.Add(25 * time.Hour)) {
		t.Fatal("an update put off should be offered again the next day")
	}
}
