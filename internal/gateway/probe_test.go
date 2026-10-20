package gateway

import (
	"testing"

	wire "opencode2api/internal/protocol"
)

// TestDocumentHasOutput guards the probe's success criterion. A well-formed but
// empty reply used to count as success on the streaming path only, which kept a
// broken model enabled until the next hourly check.
func TestDocumentHasOutput(t *testing.T) {
	cases := []struct {
		name     string
		protocol wire.Protocol
		document map[string]any
		want     bool
	}{
		{"chat text", wire.Chat, map[string]any{"choices": []any{
			map[string]any{"message": map[string]any{"content": "OK"}},
		}}, true},
		{"chat empty", wire.Chat, map[string]any{"choices": []any{
			map[string]any{"message": map[string]any{"content": ""}},
		}}, false},
		{"chat blank", wire.Chat, map[string]any{"choices": []any{
			map[string]any{"message": map[string]any{"content": "   "}},
		}}, false},
		{"chat tool call", wire.Chat, map[string]any{"choices": []any{
			map[string]any{"message": map[string]any{"content": nil, "tool_calls": []any{map[string]any{"id": "call_1"}}}},
		}}, true},
		{"chat no choices", wire.Chat, map[string]any{}, false},
		{"anthropic text", wire.Anthropic, map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "OK"},
		}}, true},
		{"anthropic empty", wire.Anthropic, map[string]any{"content": []any{
			map[string]any{"type": "text", "text": ""},
		}}, false},
		{"anthropic tool", wire.Anthropic, map[string]any{"content": []any{
			map[string]any{"type": "tool_use", "id": "toolu_1"},
		}}, true},
		{"responses text", wire.Responses, map[string]any{"output": []any{
			map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "OK"}}},
		}}, true},
		{"responses empty", wire.Responses, map[string]any{"output": []any{
			map[string]any{"type": "message", "content": []any{}},
		}}, false},
		{"responses function call", wire.Responses, map[string]any{"output": []any{
			map[string]any{"type": "function_call", "name": "lookup"},
		}}, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := documentHasOutput(test.protocol, test.document); got != test.want {
				t.Errorf("documentHasOutput(%s) = %v, want %v", test.protocol, got, test.want)
			}
		})
	}
}
