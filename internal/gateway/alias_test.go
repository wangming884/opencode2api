package gateway

import (
	"strings"
	"testing"

	"opencode2api/internal/config"
	modelcatalog "opencode2api/internal/models"
)

// A rejected model ID has two very different causes, and only one of them is
// fixed by checking the spelling. The catalog knows the difference, so the
// message must report it instead of always blaming the protocol.
func TestUnavailableMessage(t *testing.T) {
	catalog := modelcatalog.Catalog(config.TierZen, nil)
	catalog.Replace([]string{"listed-model"}, nil)
	g := &Gateway{catalog: catalog}

	cases := []struct {
		requested, served string
		want              []string
		notWant           string
	}{
		{"listed-model", "listed-model", []string{"upstream protocol"}, ""},
		{"ghost", "ghost", []string{"not offered by the configured Zen or Go pools"}, "upstream protocol"},
		{"gpt-4o", "ghost", []string{`"gpt-4o" is aliased to "ghost"`, "not offered"}, "upstream protocol"},
		{"gpt-4o", "listed-model", []string{`"gpt-4o" is aliased to "listed-model"`, "upstream protocol"}, ""},
	}
	for _, test := range cases {
		got := g.unavailableMessage(test.requested, test.served)
		for _, want := range test.want {
			if !strings.Contains(got, want) {
				t.Errorf("unavailableMessage(%q, %q) = %q, want it to contain %q", test.requested, test.served, got, want)
			}
		}
		if test.notWant != "" && strings.Contains(got, test.notWant) {
			t.Errorf("unavailableMessage(%q, %q) = %q, want no mention of %q", test.requested, test.served, got, test.notWant)
		}
	}
}

// The verbatim relays (System One decisions, embeddings) carry the caller's
// bytes upstream, so an alias has to be applied by re-encoding the body. A
// plain re-decode would turn a large integer into a float and silently change
// the request.
func TestRewriteModelBody(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","seed":9007199254740993,"max_tokens":16,"stream":true}`)
	out, err := rewriteModelBody(body, "glm-5.3")
	if err != nil {
		t.Fatalf("rewriteModelBody: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, `"model":"glm-5.3"`) {
		t.Errorf("model was not replaced: %s", text)
	}
	if !strings.Contains(text, "9007199254740993") {
		t.Errorf("a large integer lost precision: %s", text)
	}
	if !strings.Contains(text, `"max_tokens":16`) || !strings.Contains(text, `"stream":true`) {
		t.Errorf("other fields were not preserved: %s", text)
	}
	if _, err := rewriteModelBody([]byte(`[]`), "glm-5.3"); err == nil {
		t.Error("a non-object body was accepted")
	}
}
