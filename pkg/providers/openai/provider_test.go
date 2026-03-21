package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/argobell/clawcord/pkg/providers"
)

func TestProviderChatUsesMaxCompletionTokensForGPT5(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"content": "ok"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	provider := NewProvider("key", server.URL, "")
	maxTokens := int64(1234)
	_, err := provider.Chat(
		context.Background(),
		[]providers.Message{{Role: "user", Content: "hi"}},
		nil,
		"gpt-5.4",
		map[string]any{"max_tokens": maxTokens},
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if _, ok := requestBody["max_completion_tokens"]; !ok {
		t.Fatal("expected max_completion_tokens in request body")
	}
	if _, ok := requestBody["max_tokens"]; ok {
		t.Fatal("did not expect max_tokens in request body")
	}
}

func TestProviderChatOmitPromptCacheKeyForNonOpenAI(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"content": "ok"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	provider := NewProvider("key", server.URL, "")
	_, err := provider.Chat(
		context.Background(),
		[]providers.Message{{Role: "user", Content: "hi"}},
		nil,
		"deepseek/deepseek-chat",
		map[string]any{"prompt_cache_key": "agent:main"},
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if _, ok := requestBody["prompt_cache_key"]; ok {
		t.Fatal("prompt_cache_key should be omitted for non-OpenAI endpoints")
	}
	if got := requestBody["model"]; got != "deepseek-chat" {
		t.Fatalf("model = %v, want deepseek-chat", got)
	}
}

func TestProviderChatNormalizesKimiTemperature(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"content": "ok"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	temp := 0.1
	provider := NewProvider("key", server.URL, "")
	_, err := provider.Chat(
		context.Background(),
		[]providers.Message{{Role: "user", Content: "hi"}},
		nil,
		"moonshot/kimi-k2",
		map[string]any{"temperature": temp},
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if got := requestBody["model"]; got != "kimi-k2" {
		t.Fatalf("model = %v, want kimi-k2", got)
	}
	if got := requestBody["temperature"]; got != float64(1) {
		t.Fatalf("temperature = %v, want 1", got)
	}
}

func TestProviderChatParsesToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": "",
					"tool_calls": []map[string]any{{
						"id":   "call_1",
						"type": "function",
						"function": map[string]any{
							"name":      "get_weather",
							"arguments": "{\"city\":\"SF\"}",
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer server.Close()

	provider := NewProvider("key", server.URL, "")
	out, err := provider.Chat(context.Background(), []providers.Message{{Role: "user", Content: "hi"}}, nil, "gpt-4o", nil)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
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
	if out.Usage == nil || out.Usage.TotalTokens != 15 {
		t.Fatalf("Usage = %+v, want total_tokens=15", out.Usage)
	}
}

func TestProviderChatReturnsHelpfulHTMLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html><body>gateway error</body></html>"))
	}))
	defer server.Close()

	provider := NewProvider("key", server.URL, "")
	_, err := provider.Chat(
		context.Background(),
		[]providers.Message{{Role: "user", Content: "hi"}},
		nil,
		"gpt-5.4",
		nil,
	)
	if err == nil {
		t.Fatal("expected error for HTML response")
	}
	if !strings.Contains(err.Error(), "returned HTML instead of JSON") {
		t.Fatalf("error = %q, want HTML hint", err.Error())
	}
}

func TestProviderChatAcceptsNumericOptionTypes(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"content": "ok"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	provider := NewProvider("key", server.URL, "")
	_, err := provider.Chat(
		context.Background(),
		[]providers.Message{{Role: "user", Content: "hi"}},
		nil,
		"gpt-4o",
		map[string]any{
			"max_tokens":  256.0,
			"temperature": float32(0.75),
		},
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if got := requestBody["max_tokens"]; got != float64(256) {
		t.Fatalf("max_tokens = %v, want 256", got)
	}
	if got := requestBody["temperature"]; got != float64(0.75) {
		t.Fatalf("temperature = %v, want 0.75", got)
	}
}

func TestNewProviderWithMaxTokensFieldAndTimeout(t *testing.T) {
	provider := NewProvider(
		"key",
		"https://api.openai.com/v1",
		"http://127.0.0.1:7890",
		WithMaxTokensField("max_completion_tokens"),
		WithRequestTimeout(45*time.Second),
	)

	if provider.httpClient == nil {
		t.Fatal("httpClient should not be nil")
	}
	if provider.httpClient.Timeout != 45*time.Second {
		t.Fatalf("httpClient.Timeout = %v, want 45s", provider.httpClient.Timeout)
	}
	if provider.maxTokensField != "max_completion_tokens" {
		t.Fatalf("maxTokensField = %q, want max_completion_tokens", provider.maxTokensField)
	}
}

func TestNewProviderWithMaxTokensFieldAndTimeoutConvenience(t *testing.T) {
	provider := NewProviderWithMaxTokensFieldAndTimeout(
		"key",
		"https://api.openai.com/v1",
		"http://127.0.0.1:7890",
		"max_completion_tokens",
		300,
	)
	if provider.httpClient.Timeout != 300*time.Second {
		t.Fatalf("httpClient.Timeout = %v, want 300s", provider.httpClient.Timeout)
	}
	if provider.maxTokensField != "max_completion_tokens" {
		t.Fatalf("maxTokensField = %q, want max_completion_tokens", provider.maxTokensField)
	}
}

func TestSupportsPromptCacheKey(t *testing.T) {
	tests := []struct {
		apiBase string
		want    bool
	}{
		{apiBase: "https://api.openai.com/v1", want: true},
		{apiBase: "https://foo.openai.azure.com/openai/deployments/bar", want: true},
		{apiBase: "https://api.deepseek.com/v1", want: false},
		{apiBase: "://bad", want: false},
	}

	for _, tt := range tests {
		if got := supportsPromptCacheKey(tt.apiBase); got != tt.want {
			t.Fatalf("supportsPromptCacheKey(%q) = %v, want %v", tt.apiBase, got, tt.want)
		}
	}
}

func TestNormalizeModel(t *testing.T) {
	tests := []struct {
		model   string
		apiBase string
		want    string
	}{
		{model: "deepseek/deepseek-chat", apiBase: "https://api.deepseek.com/v1", want: "deepseek-chat"},
		{model: "moonshot/kimi-k2", apiBase: "https://api.moonshot.cn/v1", want: "kimi-k2"},
		{model: "openrouter/auto", apiBase: "https://openrouter.ai/api/v1", want: "openrouter/auto"},
	}

	for _, tt := range tests {
		if got := normalizeModel(tt.model, tt.apiBase); got != tt.want {
			t.Fatalf("normalizeModel(%q, %q) = %q, want %q", tt.model, tt.apiBase, got, tt.want)
		}
	}
}

func TestNewProviderConfiguresProxyTransport(t *testing.T) {
	provider := NewProvider("key", "https://api.openai.com/v1", "http://127.0.0.1:7890")
	transport, ok := provider.httpClient.Transport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatalf("Transport = %T, want *http.Transport", provider.httpClient.Transport)
	}
	proxyURL, err := transport.Proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "api.openai.com"}})
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:7890" {
		t.Fatalf("proxyURL = %v, want http://127.0.0.1:7890", proxyURL)
	}
}
