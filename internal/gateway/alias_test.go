package gateway

import (
	"strings"
	"testing"
)

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
