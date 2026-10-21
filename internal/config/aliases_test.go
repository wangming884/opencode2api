package config

import (
	"strings"
	"testing"
)

func TestResolveModel(t *testing.T) {
	cfg := Config{Models: ModelsConfig{Aliases: map[string]string{
		"gpt-4o":          "glm-5.3",
		"claude-sonnet-5": "gpt-4o",
		"sota-fast":       "sota",
	}}}
	cases := map[string]string{
		"glm-5.3":         "glm-5.3", // not an alias: resolves to itself
		"gpt-4o":          "glm-5.3",
		"claude-sonnet-5": "glm-5.3", // chain: two hops
		"sota-fast":       "sota",    // an alias may point at a rotation group
		"":                "",
	}
	for model, want := range cases {
		if got := cfg.ResolveModel(model); got != want {
			t.Errorf("ResolveModel(%q) = %q, want %q", model, got, want)
		}
	}
	if !cfg.Aliased("gpt-4o") || cfg.Aliased("glm-5.3") {
		t.Error("Aliased did not distinguish a public name from a real model")
	}
}

// A loop cannot reach the gateway through Normalize, but a Config built in
// code can carry one; the walk must terminate rather than hang a request.
func TestResolveModelTerminatesOnLoop(t *testing.T) {
	cfg := Config{Models: ModelsConfig{Aliases: map[string]string{"a": "b", "b": "a"}}}
	if got := cfg.ResolveModel("a"); got != "a" && got != "b" {
		t.Errorf("ResolveModel on a loop = %q, want one of the names in the loop", got)
	}
	long := map[string]string{}
	for _, pair := range [][2]string{{"m0", "m1"}, {"m1", "m2"}, {"m2", "m3"}, {"m3", "m4"}, {"m4", "m5"}, {"m5", "m6"}, {"m6", "m7"}, {"m7", "m8"}, {"m8", "m9"}} {
		long[pair[0]] = pair[1]
	}
	if got := (Config{Models: ModelsConfig{Aliases: long}}).ResolveModel("m0"); got == "" {
		t.Error("a long chain resolved to nothing")
	}
}

func TestNormalizeModelAliases(t *testing.T) {
	cfg := Config{
		Rotation: RotationConfig{SOTA: RotationGroup{Alias: "sota"}, Sweet: RotationGroup{Alias: "sweet"}},
		Models: ModelsConfig{
			Aliases: map[string]string{" gpt-4o ": " glm-5.3 ", "claude-sonnet-5": "gpt-4o"},
			Hidden:  []string{" glm-5.3 ", "glm-5.3", "", "kimi-k3"},
		},
	}
	if err := normalizeModelAliases(&cfg); err != nil {
		t.Fatalf("normalizeModelAliases: %v", err)
	}
	if got := cfg.Models.Aliases["gpt-4o"]; got != "glm-5.3" {
		t.Errorf("alias key or target was not trimmed: %q", got)
	}
	if len(cfg.Models.Hidden) != 2 {
		t.Errorf("hidden = %v, want the trimmed, deduplicated pair", cfg.Models.Hidden)
	}
	if !cfg.Models.Hides("glm-5.3") || cfg.Models.Hides("gpt-4o") {
		t.Error("Hides did not report the configured models")
	}
}

func TestNormalizeModelAliasesRejects(t *testing.T) {
	cases := []struct {
		name    string
		aliases map[string]string
		want    string
	}{
		{"self", map[string]string{"gpt-4o": "gpt-4o"}, "to itself"},
		{"empty target", map[string]string{"gpt-4o": "  "}, "non-empty"},
		{"empty name", map[string]string{"": "glm-5.3"}, "non-empty"},
		{"loop", map[string]string{"a": "b", "b": "c", "c": "a"}, "loop"},
		{"rotation alias", map[string]string{"sota": "glm-5.3"}, "rotation alias"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cfg := Config{
				Rotation: RotationConfig{SOTA: RotationGroup{Alias: "sota"}, Sweet: RotationGroup{Alias: "sweet"}},
				Models:   ModelsConfig{Aliases: test.aliases},
			}
			err := normalizeModelAliases(&cfg)
			if err == nil {
				t.Fatalf("accepted %v", test.aliases)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error %q does not mention %q", err, test.want)
			}
		})
	}
}

func TestServerKeyAllowsRequest(t *testing.T) {
	cfg := Config{ServerKeyMetadata: map[string]ServerKeyMetadata{
		"public":  {Limits: &ServerKeyLimits{Models: []string{"gpt-*"}}},
		"private": {Limits: &ServerKeyLimits{Models: []string{"glm-5.3"}}},
		"open":    {},
	}}
	cases := []struct {
		id                string
		requested, served string
		want              bool
		why               string
	}{
		{"public", "gpt-4o", "glm-5.3", true, "the policy names the public pattern"},
		{"private", "gpt-4o", "glm-5.3", true, "the policy names the real model"},
		{"private", "gpt-4o", "kimi-k3", false, "neither name is allowed"},
		{"open", "anything", "anything-else", true, "a key without a model list is unlimited"},
	}
	for _, test := range cases {
		if got := cfg.ServerKeyAllowsRequest(test.id, test.requested, test.served); got != test.want {
			t.Errorf("ServerKeyAllowsRequest(%q, %q, %q) = %v, want %v (%s)", test.id, test.requested, test.served, got, test.want, test.why)
		}
	}
}
