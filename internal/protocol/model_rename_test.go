package protocol

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// An aliased model must not be disclosed in the body: the caller sees the name
// it asked for, in every protocol shape.
func TestRenameResponseModel(t *testing.T) {
	cases := []struct {
		name     string
		protocol Protocol
		body     string
		want     string
	}{
		{
			name:     "chat",
			protocol: Chat,
			body:     `{"id":"c1","model":"glm-5.3","choices":[{"message":{"content":"ok"}}]}`,
			want:     "gpt-4o",
		},
		{
			name:     "anthropic nests the model under message",
			protocol: Anthropic,
			body:     `{"type":"message_start","message":{"model":"glm-5.3","content":[]}}`,
			want:     "gpt-4o",
		},
		{
			name:     "responses event wraps the document",
			protocol: Responses,
			body:     `{"type":"response.created","response":{"model":"glm-5.3","output":[]}}`,
			want:     "gpt-4o",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			out := RenameResponseModel([]byte(test.body), test.protocol, "glm-5.3", "gpt-4o")
			if !strings.Contains(string(out), `"`+test.want+`"`) {
				t.Fatalf("model was not renamed: %s", out)
			}
			if strings.Contains(string(out), "glm-5.3") {
				t.Errorf("the served model is still in the body: %s", out)
			}
			var doc map[string]any
			if err := json.Unmarshal(out, &doc); err != nil {
				t.Errorf("renaming produced invalid JSON: %v", err)
			}
		})
	}
}

func TestRenameResponseModelLeavesOtherBodiesAlone(t *testing.T) {
	body := []byte(`{"id":"c1","model":"kimi-k3"}`)
	if out := RenameResponseModel(body, Chat, "glm-5.3", "gpt-4o"); string(out) != string(body) {
		t.Errorf("a body naming another model was rewritten: %s", out)
	}
	if out := RenameResponseModel(body, Chat, "kimi-k3", "kimi-k3"); string(out) != string(body) {
		t.Errorf("an identical rename rewrote the body: %s", out)
	}
}

func TestRenameStreamModel(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"c1","model":"glm-5.3","choices":[{"delta":{"content":"He"}}]}`,
		"",
		`data: {"id":"c1","model":"glm-5.3","choices":[{"delta":{"content":"llo"}}]}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")
	out, err := io.ReadAll(RenameStreamModel(strings.NewReader(stream), Chat, "glm-5.3", "gpt-4o"))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if strings.Contains(string(out), "glm-5.3") {
		t.Errorf("the served model survived in the stream: %s", out)
	}
	if strings.Count(string(out), `"model":"gpt-4o"`) != 2 {
		t.Errorf("expected both frames renamed, got: %s", out)
	}
	if !strings.HasSuffix(string(out), "data: [DONE]\n") {
		t.Errorf("the terminator was not preserved: %q", out)
	}
}
