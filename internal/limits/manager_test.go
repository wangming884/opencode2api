package limits

import (
	"testing"
	"time"

	"opencode2api/internal/config"
)

// fixedClock returns a clock the test can move forward.
func fixedClock(start time.Time) (func() time.Time, func(time.Duration)) {
	now := start
	return func() time.Time { return now }, func(step time.Duration) { now = now.Add(step) }
}

func TestAllowEnforcesRollingMinute(t *testing.T) {
	clock, advance := fixedClock(time.Unix(1_700_000_000, 0))
	manager := newManager(clock)
	policy := config.ServerKeyLimits{RPM: 2}

	for i := 0; i < 2; i++ {
		if decision := manager.Allow("key", policy); !decision.Allowed {
			t.Fatalf("request %d refused: %s", i+1, decision.Message)
		}
	}
	decision := manager.Allow("key", policy)
	if decision.Allowed {
		t.Fatal("third request in the same minute was admitted")
	}
	if decision.Reason != ReasonRate {
		t.Fatalf("reason = %q, want %q", decision.Reason, ReasonRate)
	}
	if decision.RetryAfter < 1 || decision.RetryAfter > 60 {
		t.Fatalf("retry_after = %d, want 1..60", decision.RetryAfter)
	}

	advance(61 * time.Second)
	if decision := manager.Allow("key", policy); !decision.Allowed {
		t.Fatalf("window did not roll over: %s", decision.Message)
	}
}

func TestAllowIsPerKey(t *testing.T) {
	clock, _ := fixedClock(time.Unix(1_700_000_000, 0))
	manager := newManager(clock)
	policy := config.ServerKeyLimits{RPM: 1}

	if decision := manager.Allow("first", policy); !decision.Allowed {
		t.Fatal("first key refused")
	}
	if decision := manager.Allow("first", policy); decision.Allowed {
		t.Fatal("first key exceeded its own limit")
	}
	if decision := manager.Allow("second", policy); !decision.Allowed {
		t.Fatal("one key's traffic limited another key")
	}
}

func TestAllowEnforcesDailyRequestsAndResetsAtMidnight(t *testing.T) {
	// One second before a UTC midnight boundary.
	start := time.Unix(1_700_006_400-1, 0)
	clock, advance := fixedClock(start)
	manager := newManager(clock)
	policy := config.ServerKeyLimits{DailyRequests: 1}

	if decision := manager.Allow("key", policy); !decision.Allowed {
		t.Fatal("first request refused")
	}
	decision := manager.Allow("key", policy)
	if decision.Allowed || decision.Reason != ReasonDailyRequests {
		t.Fatalf("second request: allowed=%v reason=%q", decision.Allowed, decision.Reason)
	}
	if decision.RetryAfter != 1 {
		t.Fatalf("retry_after = %d, want 1 second to midnight", decision.RetryAfter)
	}

	advance(2 * time.Second)
	if decision := manager.Allow("key", policy); !decision.Allowed {
		t.Fatalf("daily counter did not reset: %s", decision.Message)
	}
}

func TestTokenBudgetBlocksAfterUsage(t *testing.T) {
	clock, advance := fixedClock(time.Unix(1_700_000_000, 0))
	manager := newManager(clock)
	policy := config.ServerKeyLimits{DailyTokens: 100}

	if decision := manager.Allow("key", policy); !decision.Allowed {
		t.Fatal("first request refused")
	}
	// Usage is only known after the upstream answers, so the request that
	// crosses the budget still completes.
	manager.RecordUsage("key", 120)
	if got := manager.Lookup("key").DayTokens; got != 120 {
		t.Fatalf("day tokens = %d, want 120", got)
	}
	decision := manager.Allow("key", policy)
	if decision.Allowed || decision.Reason != ReasonDailyTokens {
		t.Fatalf("budget not enforced: allowed=%v reason=%q", decision.Allowed, decision.Reason)
	}

	advance(24 * time.Hour)
	if decision := manager.Allow("key", policy); !decision.Allowed {
		t.Fatalf("budget did not reset: %s", decision.Message)
	}
}

func TestZeroPolicyIsUnlimited(t *testing.T) {
	clock, _ := fixedClock(time.Unix(1_700_000_000, 0))
	manager := newManager(clock)
	for i := 0; i < 500; i++ {
		if decision := manager.Allow("key", config.ServerKeyLimits{}); !decision.Allowed {
			t.Fatalf("unlimited key refused at request %d", i+1)
		}
	}
}

func TestLookupReportsCounters(t *testing.T) {
	clock, _ := fixedClock(time.Unix(1_700_000_000, 0))
	manager := newManager(clock)
	if snapshot := manager.Lookup("unknown"); snapshot.DayRequests != 0 {
		t.Fatalf("unknown key reported %+v", snapshot)
	}
	manager.Allow("key", config.ServerKeyLimits{RPM: 5})
	manager.RecordUsage("key", 7)
	snapshot := manager.Lookup("key")
	if snapshot.MinuteRequests != 1 || snapshot.DayRequests != 1 || snapshot.DayTokens != 7 {
		t.Fatalf("snapshot = %+v, want 1 request and 7 tokens", snapshot)
	}
}

func TestReleaseReturnsAnAdmittedSlot(t *testing.T) {
	clock, _ := fixedClock(time.Unix(1_700_000_000, 0))
	manager := newManager(clock)
	policy := config.ServerKeyLimits{RPM: 1}

	if decision := manager.Allow("key", policy); !decision.Allowed {
		t.Fatal("first request refused")
	}
	if decision := manager.Allow("key", policy); decision.Allowed {
		t.Fatal("second request was admitted past the limit")
	}
	// A model rejected by the allowlist is not the caller's traffic budget.
	manager.Release("key")
	if decision := manager.Allow("key", policy); !decision.Allowed {
		t.Fatal("released slot was not returned")
	}
	if got := manager.Lookup("key").DayRequests; got != 1 {
		t.Fatalf("day requests = %d, want 1", got)
	}
}

func TestReleaseOnUnknownKeyIsNoop(t *testing.T) {
	clock, _ := fixedClock(time.Unix(1_700_000_000, 0))
	manager := newManager(clock)
	manager.Release("missing")
	if snapshot := manager.Lookup("missing"); snapshot.DayRequests != 0 {
		t.Fatalf("snapshot = %+v, want zeroes", snapshot)
	}
}

func TestRecordUsageIgnoresNonPositive(t *testing.T) {
	clock, _ := fixedClock(time.Unix(1_700_000_000, 0))
	manager := newManager(clock)
	manager.RecordUsage("key", 0)
	manager.RecordUsage("key", -5)
	if got := manager.Lookup("key").DayTokens; got != 0 {
		t.Fatalf("day tokens = %d, want 0", got)
	}
}
