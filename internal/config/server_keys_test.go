package config

import (
	"testing"
	"time"
)

func TestServerKeyExpiry(t *testing.T) {
	past := time.Now().Add(-time.Minute).UTC()
	future := time.Now().Add(time.Hour).UTC()
	cfg := Config{
		ServerKeys: []string{"expired-key", "live-key", "off-key"},
		ServerKeyMetadata: map[string]ServerKeyMetadata{
			Fingerprint("expired-key"): {ExpiresAt: &past},
			Fingerprint("live-key"):    {ExpiresAt: &future},
			Fingerprint("off-key"):     {Disabled: true},
		},
	}
	if !cfg.ServerKeyExpired(Fingerprint("expired-key")) {
		t.Error("a past expiry was not reported as expired")
	}
	if cfg.ServerKeyExpired(Fingerprint("live-key")) {
		t.Error("a future expiry was reported as expired")
	}
	if cfg.ServerKeyExpired(Fingerprint("off-key")) {
		t.Error("a key without an expiry was reported as expired")
	}
	if cfg.ServerKeyExpired("unknown") {
		t.Error("an unknown key was reported as expired")
	}
}

func TestServerKeyAllowsModel(t *testing.T) {
	cfg := Config{ServerKeyMetadata: map[string]ServerKeyMetadata{
		"restricted": {Limits: &ServerKeyLimits{Models: []string{"gpt-4o", "gpt-*", "claude-opus-5"}}},
		"open":       {},
	}}
	cases := []struct {
		id    string
		model string
		want  bool
	}{
		{"restricted", "gpt-4o", true},
		{"restricted", "gpt-4o-mini", true},
		{"restricted", "claude-opus-5", true},
		{"restricted", "claude-sonnet-5", false},
		{"restricted", "deepseek-v4.1-flash", false},
		{"open", "anything", true},
		{"unknown", "anything", true},
	}
	for _, test := range cases {
		if got := cfg.ServerKeyAllowsModel(test.id, test.model); got != test.want {
			t.Errorf("ServerKeyAllowsModel(%q, %q) = %v, want %v", test.id, test.model, got, test.want)
		}
	}
}

func TestNormalizeServerKeyLimits(t *testing.T) {
	cfg := Config{ServerKeyMetadata: map[string]ServerKeyMetadata{
		"empty":    {Limits: &ServerKeyLimits{}},
		"tidied":   {Limits: &ServerKeyLimits{RPM: 10, Models: []string{" a ", "", "a", "b"}}},
		"negative": {Limits: &ServerKeyLimits{RPM: -1}},
	}}

	// A negative ceiling is a configuration error, not a silent "unlimited".
	if err := normalizeServerKeyLimits(&cfg); err == nil {
		t.Fatal("negative limit was accepted")
	}

	delete(cfg.ServerKeyMetadata, "negative")
	if err := normalizeServerKeyLimits(&cfg); err != nil {
		t.Fatalf("normalizeServerKeyLimits: %v", err)
	}
	if cfg.ServerKeyMetadata["empty"].Limits != nil {
		t.Error("a policy that limits nothing was kept")
	}
	tidied := cfg.ServerKeyMetadata["tidied"].Limits
	if tidied == nil || len(tidied.Models) != 2 || tidied.Models[0] != "a" || tidied.Models[1] != "b" {
		t.Errorf("models = %+v, want [a b]", tidied)
	}
}

func TestCloneDeepCopiesLimits(t *testing.T) {
	original := Config{ServerKeyMetadata: map[string]ServerKeyMetadata{
		"id": {Limits: &ServerKeyLimits{RPM: 5, Models: []string{"a"}}},
	}}
	copied := Clone(original)
	copied.ServerKeyMetadata["id"].Limits.Models[0] = "changed"
	copied.ServerKeyMetadata["id"].Limits.RPM = 99

	if original.ServerKeyMetadata["id"].Limits.Models[0] != "a" || original.ServerKeyMetadata["id"].Limits.RPM != 5 {
		t.Fatal("Clone shared the policy with the original config")
	}
}
