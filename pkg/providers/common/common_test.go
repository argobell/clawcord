package common

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/argobell/clawcord/pkg/providers/protocoltypes"
)

func TestSerializeMessagesStripsSystemPartsAndPreservesReasoning(t *testing.T) {
	messages := []Message{
		{
			Role:    "system",
			Content: "you are helpful",
			SystemParts: []protocoltypes.ContentBlock{
				{Type: "text", Text: "hidden"},
			},
		},
		{
			Role:             "assistant",
			Content:          "2",
			ReasoningContent: "1+1=2",
		},
	}

	result := SerializeMessages(messages)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if strings.Contains(string(data), "system_parts") {
		t.Fatal("system_parts should not appear in serialized output")
	}
	if !strings.Contains(string(data), "reasoning_content") {
		t.Fatal("reasoning_content should be preserved in serialized output")
	}
}

func TestSerializeMessagesWithMediaAndToolCallID(t *testing.T) {
	messages := []Message{
		{
			Role:       "tool",
			Content:    "result",
			Media:      []string{"data:image/png;base64,abc123"},
			ToolCallID: "call_1",
		},
	}

	result := SerializeMessages(messages)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var msgs []map[string]any
	if err := json.Unmarshal(data, &msgs); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	content, ok := msgs[0]["content"].([]any)
	if !ok {
		t.Fatalf("content = %T, want []any", msgs[0]["content"])
	}
	if len(content) != 2 {
		t.Fatalf("len(content) = %d, want 2", len(content))
	}
	if msgs[0]["tool_call_id"] != "call_1" {
		t.Fatalf("tool_call_id = %v, want call_1", msgs[0]["tool_call_id"])
	}
}

func TestSerializeMessagesWithAudioDataURL(t *testing.T) {
	messages := []Message{
		{
			Role:  "user",
			Media: []string{"data:audio/mp3;base64,ZmFrZQ=="},
		},
	}

	result := SerializeMessages(messages)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var msgs []map[string]any
	if err := json.Unmarshal(data, &msgs); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	content, ok := msgs[0]["content"].([]any)
	if !ok {
		t.Fatalf("content = %T, want []any", msgs[0]["content"])
	}
	if len(content) != 1 {
		t.Fatalf("len(content) = %d, want 1", len(content))
	}

	part, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("content[0] = %T, want map[string]any", content[0])
	}
	if part["type"] != "input_audio" {
		t.Fatalf("type = %v, want input_audio", part["type"])
	}

	inputAudio, ok := part["input_audio"].(map[string]any)
	if !ok {
		t.Fatalf("input_audio = %T, want map[string]any", part["input_audio"])
	}
	if inputAudio["format"] != "mp3" {
		t.Fatalf("format = %v, want mp3", inputAudio["format"])
	}
	if inputAudio["data"] != "ZmFrZQ==" {
		t.Fatalf("data = %v, want ZmFrZQ==", inputAudio["data"])
	}
}

func TestParseResponseDecodesToolCallArguments(t *testing.T) {
	body := `{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`

	out, err := ParseResponse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}

	if len(out.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(out.ToolCalls))
	}
	if out.ToolCalls[0].Name != "get_weather" {
		t.Fatalf("ToolCalls[0].Name = %q, want get_weather", out.ToolCalls[0].Name)
	}
	if out.ToolCalls[0].Arguments["city"] != "SF" {
		t.Fatalf("ToolCalls[0].Arguments[city] = %v, want SF", out.ToolCalls[0].Arguments["city"])
	}
	if out.Usage == nil || out.Usage.TotalTokens != 12 {
		t.Fatalf("Usage = %+v, want total_tokens=12", out.Usage)
	}
}

func TestParseResponseDecodesObjectToolCallArguments(t *testing.T) {
	body := `{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":{"city":"SF","metric":true}}}]},"finish_reason":"tool_calls"}]}`

	out, err := ParseResponse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}
	if len(out.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(out.ToolCalls))
	}
	if out.ToolCalls[0].Arguments["city"] != "SF" {
		t.Fatalf("city = %v, want SF", out.ToolCalls[0].Arguments["city"])
	}
	if out.ToolCalls[0].Arguments["metric"] != true {
		t.Fatalf("metric = %v, want true", out.ToolCalls[0].Arguments["metric"])
	}
}

func TestParseResponseNormalizesLengthFinishReason(t *testing.T) {
	body := `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`

	out, err := ParseResponse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}
	if out.FinishReason != "truncated" {
		t.Fatalf("FinishReason = %q, want truncated", out.FinishReason)
	}
}

func TestDecodeToolCallArgumentsPreservesRawOnInvalidJSONString(t *testing.T) {
	raw := json.RawMessage(`"{not-json}"`)

	args := DecodeToolCallArguments(raw, "broken_tool")

	if args["raw"] != "{not-json}" {
		t.Fatalf("raw = %v, want {not-json}", args["raw"])
	}
}

func TestReadAndParseResponseRejectsHTML(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Header: http.Header{
			"Content-Type": []string{"text/html"},
		},
		Body: io.NopCloser(strings.NewReader("<html><body>proxy error</body></html>")),
	}

	_, err := ReadAndParseResponse(resp, "https://example.test")
	if err == nil {
		t.Fatal("expected error for HTML response")
	}
	if !strings.Contains(err.Error(), "returned HTML instead of JSON") {
		t.Fatalf("error = %q, want HTML hint", err.Error())
	}
}

func TestReadAndParseResponseParsesJSONBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`)),
	}

	out, err := ReadAndParseResponse(resp, "https://example.test")
	if err != nil {
		t.Fatalf("ReadAndParseResponse() error = %v", err)
	}
	if out.Content != "hello" {
		t.Fatalf("Content = %q, want hello", out.Content)
	}
}

func TestHandleErrorResponseIncludesPreviewForJSONBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(`{"error":"upstream failed"}`)),
	}

	err := HandleErrorResponse(resp, "https://example.test")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Status: 502") {
		t.Fatalf("error = %q, want status preview", err.Error())
	}
	if !strings.Contains(err.Error(), `{"error":"upstream failed"}`) {
		t.Fatalf("error = %q, want body preview", err.Error())
	}
}
