package protocoltypes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolCallMarshalOmitsNormalizedFields(t *testing.T) {
	toolCall := ToolCall{
		ID:   "call_1",
		Type: "function",
		Function: &FunctionCall{
			Name:      "get_weather",
			Arguments: "{\"city\":\"SF\"}",
		},
		Name: "get_weather",
		Arguments: map[string]any{
			"city": "SF",
		},
	}

	data, err := json.Marshal(toolCall)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	jsonText := string(data)
	if strings.Contains(jsonText, "\"Name\"") || strings.Contains(jsonText, "\"Arguments\"") {
		t.Fatalf("JSON should not contain exported field names: %s", jsonText)
	}
	if strings.Contains(jsonText, "\"city\":\"SF\"") && strings.Contains(jsonText, "\"arguments\":{\"city\":\"SF\"}") {
		t.Fatalf("JSON should not contain normalized arguments object: %s", jsonText)
	}
	if !strings.Contains(jsonText, "\"function\"") {
		t.Fatalf("JSON should keep function wire field: %s", jsonText)
	}
}
