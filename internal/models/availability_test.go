package models

import (
	"testing"
	"time"
)

// TestSetManualClearsChannel guards the console's "channel / reason" column: a
// manual switch is not the result of a probe, so leaving the last probe's
// channel attached reads as "anonymous / manually_enabled".
func TestSetManualClearsChannel(t *testing.T) {
	registry, err := AvailabilityStore("")
	if err != nil {
		t.Fatalf("AvailabilityStore: %v", err)
	}
	now := time.Now().UTC()
	if err := registry.Record("model", 0, true, "probe_succeeded", "anonymous", now, EffortProbe{}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if got := registry.Get("model"); got.Channel != "anonymous" || got.Reason != "probe_succeeded" {
		t.Fatalf("record = %+v, want the probe's channel and reason", got)
	}

	if err := registry.SetManual("model", true, now); err != nil {
		t.Fatalf("SetManual(true): %v", err)
	}
	if got := registry.Get("model"); got.Channel != "" || got.Reason != "manually_enabled" || !got.Manual {
		t.Fatalf("after manual switch: %+v", got)
	}

	if err := registry.SetManual("model", false, now); err != nil {
		t.Fatalf("SetManual(false): %v", err)
	}
	if got := registry.Get("model"); got.Channel != "" || got.Reason != "auto_probe" || got.Manual {
		t.Fatalf("after returning to automatic: %+v", got)
	}
}

// TestRecordDropsStaleGeneration guards the probe/manual race: a probe that
// started before a manual switch must not overwrite the operator's decision.
func TestRecordDropsStaleGeneration(t *testing.T) {
	registry, err := AvailabilityStore("")
	if err != nil {
		t.Fatalf("AvailabilityStore: %v", err)
	}
	now := time.Now().UTC()
	if err := registry.SetManual("model", true, now); err != nil {
		t.Fatalf("SetManual: %v", err)
	}
	before := registry.Get("model")

	// Generation 0 is the value a probe read before the switch.
	if err := registry.Record("model", 0, false, "model_unavailable", "anonymous", now, EffortProbe{}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	after := registry.Get("model")
	if after.Reason != before.Reason || after.Disabled || after.Channel != before.Channel {
		t.Fatalf("stale probe overwrote the manual state: %+v", after)
	}
}
