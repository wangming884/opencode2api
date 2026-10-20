// Package limits enforces per-server-key traffic policy: a rolling request
// rate, a daily request ceiling, a daily token budget and a model allowlist.
//
// The manager owns only the counters; the policy itself comes from the
// configuration on every call, so editing a key's limits needs no reset and
// takes effect on the next request. Counters survive configuration reloads
// because the runtime controller shares one manager across gateways, but they
// live in memory: a restart starts every key's day over.
package limits

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"opencode2api/internal/config"
)

// Reasons a decision refuses a request. They name the policy field that
// stopped it, so logs and clients can tell a rate limit from an exhausted
// budget.
const (
	ReasonRate          = "rpm"
	ReasonDailyRequests = "daily_requests"
	ReasonDailyTokens   = "daily_tokens"
)

// Decision is the outcome of a policy check.
type Decision struct {
	Allowed bool
	Reason  string
	Message string
	// RetryAfter is the number of seconds until the window that refused the
	// request rolls over, so the caller can be told when to try again.
	RetryAfter int
}

// Snapshot is the counter view of one key, used by the management console.
type Snapshot struct {
	// MinuteRequests counts requests in the rolling minute.
	MinuteRequests int `json:"minute_requests"`
	// DayRequests and DayTokens count the current UTC day.
	DayRequests int   `json:"day_requests"`
	DayTokens   int64 `json:"day_tokens"`
}

// Manager holds the counters of every key that has been seen. It is safe for
// concurrent use.
type Manager struct {
	mu    sync.Mutex
	keys  map[string]*keyState
	now   func() time.Time
	dirty atomic.Bool
}

// keyState is the sliding window and daily totals of one key. The minute is a
// ring of one-second buckets: cheap to update, and exact enough for a request
// ceiling that only has to be approximately fair.
type keyState struct {
	minute      [60]minuteBucket
	day         int64
	dayRequests int
	dayTokens   int64
	lastUsed    int64
}

type minuteBucket struct {
	second int64
	count  int
}

// New returns an empty manager driven by the wall clock.
func New() *Manager {
	return newManager(time.Now)
}

func newManager(now func() time.Time) *Manager {
	return &Manager{keys: make(map[string]*keyState), now: now}
}

// Allow checks policy for the key fingerprint id and, when the request is
// admitted, counts it. A refusal carries the reason and how long to wait.
func (m *Manager) Allow(id string, policy config.ServerKeyLimits) Decision {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	second := now.Unix()
	state := m.stateLocked(id, second)

	if policy.RPM > 0 {
		if used, oldest := state.minuteRequests(second); used >= policy.RPM {
			wait := 60 - int(second-oldest)
			if wait < 1 {
				wait = 1
			}
			return Decision{
				Reason: ReasonRate, RetryAfter: wait,
				Message: fmt.Sprintf("request rate limit reached for this API key (%d per minute); retry in %ds", policy.RPM, wait),
			}
		}
	}
	if policy.DailyRequests > 0 && state.dayRequests >= policy.DailyRequests {
		return Decision{
			Reason: ReasonDailyRequests, RetryAfter: untilUTCMidnight(second),
			Message: fmt.Sprintf("daily request limit reached for this API key (%d per UTC day)", policy.DailyRequests),
		}
	}
	if policy.DailyTokens > 0 && state.dayTokens >= policy.DailyTokens {
		return Decision{
			Reason: ReasonDailyTokens, RetryAfter: untilUTCMidnight(second),
			Message: fmt.Sprintf("daily token budget exhausted for this API key (%d tokens per UTC day)", policy.DailyTokens),
		}
	}

	state.countRequest(second)
	m.dirty.Store(true)
	m.pruneLocked(second)
	return Decision{Allowed: true}
}

// Release gives back the slot an admitted request took. It exists for the one
// rejection that happens after admission and is not the caller's fault: a
// model refused by the key's allowlist. Without it, a client that keeps asking
// for a disallowed model would drain its own quota and then lock out its valid
// traffic. The second-level bucket may have rolled over by the time the
// rejection is known, in which case only the daily count is returned.
func (m *Manager) Release(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.keys[id]
	if !ok {
		return
	}
	second := m.now().Unix()
	state.rollDay(second)
	index := second % 60
	if state.minute[index].second == second && state.minute[index].count > 0 {
		state.minute[index].count--
	}
	if state.dayRequests > 0 {
		state.dayRequests--
	}
	m.dirty.Store(true)
}

// RecordUsage adds the tokens an admitted request actually consumed. Only
// reported usage is added, so a request the upstream never billed for does not
// eat the budget. Streaming calls reach this after the last event.
func (m *Manager) RecordUsage(id string, tokens int64) {
	if tokens <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	second := m.now().Unix()
	state := m.stateLocked(id, second)
	state.dayTokens += tokens
	state.lastUsed = second
	m.dirty.Store(true)
}

// Export returns the counters that are worth persisting: the daily totals of
// every key, keyed by fingerprint. The rolling minute is left out on purpose
// (see DayUsage).
func (m *Manager) Export() map[string]DayUsage {
	m.mu.Lock()
	defer m.mu.Unlock()
	second := m.now().Unix()
	usage := make(map[string]DayUsage, len(m.keys))
	for id, state := range m.keys {
		state.rollDay(second)
		if state.dayRequests == 0 && state.dayTokens == 0 {
			continue
		}
		usage[id] = DayUsage{Day: state.day, Requests: state.dayRequests, Tokens: state.dayTokens}
	}
	return usage
}

// Restore loads persisted daily totals. Entries from an earlier day are
// dropped rather than restored, so a restart the next morning never inherits
// yesterday's spending.
func (m *Manager) Restore(usage map[string]DayUsage) {
	if len(usage) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	second := m.now().Unix()
	day := second / 86400
	for id, entry := range usage {
		if entry.Day != day || (entry.Requests == 0 && entry.Tokens == 0) {
			continue
		}
		m.keys[id] = &keyState{day: entry.Day, dayRequests: entry.Requests, dayTokens: entry.Tokens, lastUsed: second}
	}
}

// TakeDirty reports whether the counters changed since the last call and
// clears the flag, which is what a periodic flush uses to skip idle writes.
func (m *Manager) TakeDirty() bool {
	return m.dirty.Swap(false)
}

// Lookup reports the counters of one key without changing them. A key that has
// never been seen reports zeroes.
func (m *Manager) Lookup(id string) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.keys[id]
	if !ok {
		return Snapshot{}
	}
	second := m.now().Unix()
	state.rollDay(second)
	used, _ := state.minuteRequests(second)
	return Snapshot{MinuteRequests: used, DayRequests: state.dayRequests, DayTokens: state.dayTokens}
}

// stateLocked returns the state of id, creating it and rolling its day over
// when the UTC day changed.
func (m *Manager) stateLocked(id string, second int64) *keyState {
	state, ok := m.keys[id]
	if !ok {
		state = &keyState{}
		m.keys[id] = state
	}
	state.rollDay(second)
	state.lastUsed = second
	return state
}

// pruneLocked drops counters of keys that have been idle for two days. It runs
// only once the map has grown past a bound, so a normal key set never pays for
// it.
func (m *Manager) pruneLocked(second int64) {
	const (
		pruneAbove = 4096
		idleLimit  = 2 * 86400
	)
	if len(m.keys) <= pruneAbove {
		return
	}
	for id, state := range m.keys {
		if second-state.lastUsed > idleLimit {
			delete(m.keys, id)
		}
	}
}

// rollDay resets the daily totals when the UTC day changed.
func (s *keyState) rollDay(second int64) {
	day := second / 86400
	if s.day == day {
		return
	}
	s.day = day
	s.dayRequests = 0
	s.dayTokens = 0
}

// countRequest adds one request to the rolling minute and the current day.
func (s *keyState) countRequest(second int64) {
	index := second % 60
	if s.minute[index].second != second {
		s.minute[index] = minuteBucket{second: second}
	}
	s.minute[index].count++
	s.dayRequests++
}

// minuteRequests sums the buckets of the last 60 seconds and reports the
// second of the oldest counted bucket, which is when the window starts to
// free up.
func (s *keyState) minuteRequests(second int64) (total int, oldest int64) {
	cutoff := second - 59
	oldest = second
	for _, bucket := range s.minute {
		if bucket.second < cutoff || bucket.second > second || bucket.count == 0 {
			continue
		}
		total += bucket.count
		if bucket.second < oldest {
			oldest = bucket.second
		}
	}
	return total, oldest
}

// untilUTCMidnight returns the seconds remaining until the daily counters roll
// over.
func untilUTCMidnight(second int64) int {
	remaining := 86400 - int(second%86400)
	if remaining < 1 {
		return 1
	}
	return remaining
}
