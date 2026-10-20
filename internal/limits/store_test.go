package limits

import (
	"path/filepath"
	"testing"
	"time"

	"opencode2api/internal/config"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "limits.json")
	store := NewStore(path)

	usage, err := store.Load()
	if err != nil {
		t.Fatalf("Load on a missing file: %v", err)
	}
	if len(usage) != 0 {
		t.Fatalf("missing file returned %d entries", len(usage))
	}

	want := map[string]DayUsage{
		"first":  {Day: 19676, Requests: 3, Tokens: 1200},
		"second": {Day: 19676, Requests: 1, Tokens: 7},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != len(want) || got["first"] != want["first"] || got["second"] != want["second"] {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestEmptyPathDisablesPersistence(t *testing.T) {
	store := NewStore("")
	if err := store.Save(map[string]DayUsage{"key": {Day: 1, Requests: 1}}); err != nil {
		t.Fatalf("Save with no path: %v", err)
	}
	usage, err := store.Load()
	if err != nil || len(usage) != 0 {
		t.Fatalf("Load with no path = %v, %v", usage, err)
	}
}

func TestRestoreDropsOtherDays(t *testing.T) {
	now := time.Unix(1_700_006_400, 0) // exactly a UTC midnight
	manager := newManager(func() time.Time { return now })
	today := now.Unix() / 86400

	manager.Restore(map[string]DayUsage{
		"today": {Day: today, Requests: 5, Tokens: 500},
		"stale": {Day: today - 1, Requests: 9, Tokens: 900},
		"empty": {Day: today},
	})

	if got := manager.Lookup("today"); got.DayRequests != 5 || got.DayTokens != 500 {
		t.Fatalf("today = %+v, want 5 requests and 500 tokens", got)
	}
	if got := manager.Lookup("stale"); got.DayRequests != 0 || got.DayTokens != 0 {
		t.Fatalf("yesterday's counters were restored: %+v", got)
	}
	if got := manager.Lookup("empty"); got.DayRequests != 0 {
		t.Fatalf("an empty entry created state: %+v", got)
	}
}

func TestExportSkipsIdleKeys(t *testing.T) {
	clock, _ := fixedClock(time.Unix(1_700_000_000, 0))
	manager := newManager(clock)
	manager.Allow("used", config.ServerKeyLimits{RPM: 1})
	manager.RecordUsage("used", 42)

	usage := manager.Export()
	if len(usage) != 1 || usage["used"].Requests != 1 || usage["used"].Tokens != 42 {
		t.Fatalf("export = %+v", usage)
	}
}

func TestDirtyTracksCounterChanges(t *testing.T) {
	clock, _ := fixedClock(time.Unix(1_700_000_000, 0))
	manager := newManager(clock)
	if manager.TakeDirty() {
		t.Fatal("a fresh manager reported itself dirty")
	}
	manager.Allow("key", config.ServerKeyLimits{})
	if !manager.TakeDirty() {
		t.Fatal("a counted request did not mark the counters dirty")
	}
	if manager.TakeDirty() {
		t.Fatal("TakeDirty did not clear the flag")
	}
}
