package protocol

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// These tests are the safety net for the conversion matrix: three protocols,
// both directions, plus streaming. They assert the fields a client can observe
// rather than an exact byte layout, so an encoder refactor is free as long as
// the meaning survives.

var protocols = []Protocol{Chat, Responses, Anthropic}

// requestFixtures is one representative request per protocol, all describing
// the same conversation so a conversion can be checked for what it preserved.
func requestFixtures() map[Protocol]map[string]any {
	tool := map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "lookup",
			"description": "Look up a fact",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"q": map[string]any{"type": "string"}},
				"required":   []any{"q"},
			},
		},
	}
	chatBody := map[string]any{
		"model":       "test-model",
		"stream":      true,
		"temperature": 0.3,
		"max_tokens":  256,
		"messages": []any{
			map[string]any{"role": "system", "content": "You are terse."},
			map[string]any{"role": "user", "content": "Hello"},
		},
		"tools": []any{tool},
	}
	responsesBody := map[string]any{
		"model":             "test-model",
		"stream":            true,
		"instructions":      "You are terse.",
		"input":             "Hello",
		"max_output_tokens": 256,
		"tools": []any{map[string]any{
			"type": "function", "name": "lookup", "description": "Look up a fact",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}, "required": []any{"q"}},
		}},
	}
	anthropicBody := map[string]any{
		"model":      "test-model",
		"stream":     true,
		"system":     "You are terse.",
		"max_tokens": 256,
		"messages":   []any{map[string]any{"role": "user", "content": "Hello"}},
		"tools": []any{map[string]any{
			"name": "lookup", "description": "Look up a fact",
			"input_schema": map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}, "required": []any{"q"}},
		}},
	}
	return map[Protocol]map[string]any{Chat: chatBody, Responses: responsesBody, Anthropic: anthropicBody}
}

func cloneBody(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return cloned
}

// textOf collects the conversation text of a prepared body. It walks the whole
// document and keeps the strings that sit under a text-bearing key, because
// each protocol nests them differently: a string input, a content string, or
// content parts under messages and input items.
func textOf(body map[string]any) string {
	var parts []string
	var walk func(value any, key string)
	walk = func(value any, key string) {
		switch typed := value.(type) {
		case string:
			if key == "text" || key == "content" || key == "input" {
				parts = append(parts, typed)
			}
		case []any:
			for _, item := range typed {
				walk(item, key)
			}
		case map[string]any:
			for name, item := range typed {
				walk(item, name)
			}
		}
	}
	for name, item := range body {
		walk(item, name)
	}
	return strings.Join(parts, "\n")
}

// toolNames lists the function names a prepared body declares.
func toolNames(body map[string]any) []string {
	tools, _ := body["tools"].([]any)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		entry, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := entry["name"].(string); ok {
			names = append(names, name)
			continue
		}
		if function, ok := entry["function"].(map[string]any); ok {
			if name, ok := function["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return names
}

// systemOf returns the system prompt wherever the target protocol keeps it.
func systemOf(protocol Protocol, body map[string]any) string {
	switch protocol {
	case Responses:
		value, _ := body["instructions"].(string)
		return value
	case Anthropic:
		if value, ok := body["system"].(string); ok {
			return value
		}
		// A system block list is also valid.
		if blocks, ok := body["system"].([]any); ok {
			var parts []string
			for _, block := range blocks {
				if entry, ok := block.(map[string]any); ok {
					if text, ok := entry["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
			return strings.Join(parts, " ")
		}
		return ""
	default:
		messages, _ := body["messages"].([]any)
		for _, message := range messages {
			entry, ok := message.(map[string]any)
			if !ok || entry["role"] != "system" {
				continue
			}
			if text, ok := entry["content"].(string); ok {
				return text
			}
		}
		return ""
	}
}

func maxTokensOf(protocol Protocol, body map[string]any) float64 {
	key := "max_tokens"
	if protocol == Responses {
		key = "max_output_tokens"
	}
	value, _ := body[key].(float64)
	return value
}

func TestPrepareRequestPreservesConversationAcrossProtocols(t *testing.T) {
	fixtures := requestFixtures()
	for _, from := range protocols {
		for _, to := range protocols {
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				prepared, err := PrepareRequest(from, to, cloneBody(t, fixtures[from]), "https://opencode.ai/zen")
				if err != nil {
					t.Fatalf("PrepareRequest: %v", err)
				}
				if model, _ := prepared["model"].(string); model != "test-model" {
					t.Errorf("model = %q, want test-model", model)
				}
				if stream, _ := prepared["stream"].(bool); !stream {
					t.Error("stream flag was dropped")
				}
				if text := textOf(prepared); !strings.Contains(text, "Hello") {
					t.Errorf("user text was lost: %q", text)
				}
				if system := systemOf(to, prepared); !strings.Contains(system, "You are terse.") {
					t.Errorf("system prompt was lost: %q", system)
				}
				if names := toolNames(prepared); len(names) != 1 || names[0] != "lookup" {
					t.Errorf("tools = %v, want [lookup]", names)
				}
				if got := maxTokensOf(to, prepared); got != 256 {
					t.Errorf("max tokens = %v, want 256", got)
				}
			})
		}
	}
}

// TestPrepareRequestKeepsSameProtocolBodiesIntact guards the pass-through path:
// a client using the upstream's own protocol must not have its provider-
// specific fields rewritten by the bridge.
func TestPrepareRequestKeepsSameProtocolBodiesIntact(t *testing.T) {
	fixtures := requestFixtures()
	for _, protocol := range protocols {
		t.Run(string(protocol), func(t *testing.T) {
			body := cloneBody(t, fixtures[protocol])
			body["metadata"] = map[string]any{"trace": "keep-me"}
			prepared, err := PrepareRequest(protocol, protocol, body, "https://opencode.ai/zen")
			if err != nil {
				t.Fatalf("PrepareRequest: %v", err)
			}
			metadata, _ := prepared["metadata"].(map[string]any)
			if metadata["trace"] != "keep-me" {
				t.Errorf("provider-specific field was dropped: %v", metadata)
			}
		})
	}
}

// responseFixtures is the same answer expressed in each protocol.
func responseFixtures() map[Protocol][]byte {
	return map[Protocol][]byte{
		Chat: []byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"test-model",
			"choices":[{"index":0,"message":{"role":"assistant","content":"Hi there"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`),
		Responses: []byte(`{"id":"resp-1","object":"response","created_at":1,"model":"test-model","status":"completed",
			"output":[{"type":"message","id":"msg-1","role":"assistant","status":"completed",
			"content":[{"type":"output_text","text":"Hi there","annotations":[]}]}],
			"usage":{"input_tokens":11,"output_tokens":5,"total_tokens":16}}`),
		Anthropic: []byte(`{"id":"msg-1","type":"message","role":"assistant","model":"test-model",
			"content":[{"type":"text","text":"Hi there"}],"stop_reason":"end_turn",
			"usage":{"input_tokens":11,"output_tokens":5}}`),
	}
}

// responseText digs the assistant text out of a converted response.
func responseText(protocol Protocol, body []byte) string {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	var parts []string
	switch protocol {
	case Chat:
		choices, _ := payload["choices"].([]any)
		for _, choice := range choices {
			entry, _ := choice.(map[string]any)
			message, _ := entry["message"].(map[string]any)
			if text, ok := message["content"].(string); ok {
				parts = append(parts, text)
			}
		}
	case Responses:
		output, _ := payload["output"].([]any)
		for _, item := range output {
			entry, _ := item.(map[string]any)
			content, _ := entry["content"].([]any)
			for _, part := range content {
				if block, ok := part.(map[string]any); ok {
					if text, ok := block["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
	case Anthropic:
		content, _ := payload["content"].([]any)
		for _, block := range content {
			if entry, ok := block.(map[string]any); ok {
				if text, ok := entry["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
	}
	return strings.Join(parts, "")
}

func TestConvertResponsePreservesTextAndUsage(t *testing.T) {
	fixtures := responseFixtures()
	for _, from := range protocols {
		for _, to := range protocols {
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				converted, err := ConvertResponse(from, to, fixtures[from])
				if err != nil {
					t.Fatalf("ConvertResponse: %v", err)
				}
				if text := responseText(to, converted); text != "Hi there" {
					t.Errorf("text = %q, want %q", text, "Hi there")
				}
				usage, reported := ResponseUsage(to, converted)
				if !reported {
					t.Fatal("converted response carries no usage")
				}
				if usage.Input != 11 || usage.Output != 5 {
					t.Errorf("usage = %+v, want 11 input and 5 output", usage)
				}
			})
		}
	}
}

// TestTranscodeStreamAnthropicToChat covers the direction a Claude Code client
// takes when it talks to a Chat-only upstream.
func TestTranscodeStreamAnthropicToChat(t *testing.T) {
	source := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[],"usage":{"input_tokens":11,"output_tokens":1}}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n\n")

	recorder := httptest.NewRecorder()
	usage, reported, err := TranscodeStream(t.Context(), recorder, strings.NewReader(source), Anthropic, Chat, "test-model")
	if err != nil {
		t.Fatalf("TranscodeStream: %v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"Hi"`) {
		t.Fatalf("stream lost text:\n%s", body)
	}
	if !strings.Contains(body, "[DONE]") {
		t.Fatalf("stream did not terminate with [DONE]:\n%s", body)
	}
	if !reported || usage.Input != 11 || usage.Output != 5 {
		t.Fatalf("usage = %+v reported=%v, want 11/5", usage, reported)
	}
}

// TestTranscodeStreamChatToAnthropic covers the streaming path, where a lost
// field or a malformed event is far harder to notice than in a JSON body.
func TestTranscodeStreamChatToAnthropic(t *testing.T) {
	source := strings.Join([]string{
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":"stop"}]}`,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}`,
		`data: [DONE]`,
		``,
	}, "\n\n")

	recorder := httptest.NewRecorder()
	usage, reported, err := TranscodeStream(t.Context(), recorder, strings.NewReader(source), Chat, Anthropic, "test-model")
	if err != nil {
		t.Fatalf("TranscodeStream: %v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "content_block_delta") {
		t.Fatalf("stream carried no content delta:\n%s", body)
	}
	if !strings.Contains(body, `"Hi"`) || !strings.Contains(body, `" there"`) {
		t.Fatalf("stream lost text:\n%s", body)
	}
	if !strings.Contains(body, "message_stop") {
		t.Fatalf("stream did not end the message:\n%s", body)
	}
	if !reported || usage.Input != 11 || usage.Output != 5 {
		t.Fatalf("usage = %+v reported=%v, want 11/5", usage, reported)
	}
}
