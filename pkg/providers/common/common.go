package common

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/argobell/clawcord/pkg/providers"
)

type (
	ToolCall               = providers.ToolCall
	FunctionCall           = providers.FunctionCall
	LLMResponse            = providers.LLMResponse
	UsageInfo              = providers.UsageInfo
	Message                = providers.Message
	ToolDefinition         = providers.ToolDefinition
	ToolFunctionDefinition = providers.ToolFunctionDefinition
	ContentBlock           = providers.ContentBlock
)

const DefaultRequestTimeout = 120 * time.Second

// NewHTTPClient 创建带默认超时的 HTTP 客户端；若传入代理则克隆默认 Transport 并注入代理配置。
func NewHTTPClient(proxy string) *http.Client {
	client := &http.Client{
		Timeout: DefaultRequestTimeout,
	}
	if strings.TrimSpace(proxy) == "" {
		return client
	}

	parsed, err := url.Parse(proxy)
	if err != nil {
		// 代理配置不合法时降级为直连，避免初始化阶段直接失败。
		log.Printf("providers/common: invalid proxy URL %q: %v", proxy, err)
		return client
	}

	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		tr := base.Clone()
		tr.Proxy = http.ProxyURL(parsed)
		client.Transport = tr
		return client
	}

	client.Transport = &http.Transport{Proxy: http.ProxyURL(parsed)}
	return client
}

type openAIMessage struct {
	Role             string     `json:"role"`
	Content          any        `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

// SerializeMessages 将通用消息结构转换为 OpenAI 兼容的消息数组，并按需编码多模态内容。
func SerializeMessages(messages []Message) []any {
	out := make([]any, 0, len(messages))
	for _, m := range messages {
		// 先统一规范 tool_calls，保证 name/arguments 在不同来源下结构一致。
		toolCalls := serializeToolCalls(m.ToolCalls)
		if len(m.Media) == 0 {
			msg := openAIMessage{
				Role:             m.Role,
				Content:          m.Content,
				ReasoningContent: m.ReasoningContent,
				ToolCalls:        toolCalls,
				ToolCallID:       m.ToolCallID,
			}
			out = append(out, msg)
			continue
		}

		parts := make([]map[string]any, 0, 1+len(m.Media))
		if m.Content != "" {
			parts = append(parts, map[string]any{
				"type": "text",
				"text": m.Content,
			})
		}
		for _, mediaURL := range m.Media {
			// 仅序列化当前支持的 data:image/* 载荷，其他媒体类型保持忽略。
			if strings.HasPrefix(mediaURL, "data:image/") {
				parts = append(parts, map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": mediaURL,
					},
				})
			}
		}

		msg := map[string]any{
			"role":    m.Role,
			"content": parts,
		}
		// tool 消息需要 tool_call_id 关联到对应调用结果。
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		if len(toolCalls) > 0 {
			msg["tool_calls"] = toolCalls
		}
		if m.ReasoningContent != "" {
			msg["reasoning_content"] = m.ReasoningContent
		}
		out = append(out, msg)
	}
	return out
}

// serializeToolCalls 将内部 ToolCall 归一为 OpenAI 函数调用格式。
//
// 参数优先级：
// 1) 使用 tc.Name / tc.Arguments（结构化字段）；
// 2) 若缺失则回退到 tc.Function 内的字符串字段；
// 3) 最终确保 Arguments 始终是 JSON 字符串，最差为 "{}"。
func serializeToolCalls(toolCalls []ToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		name := tc.Name
		arguments := tc.Arguments
		if tc.Function != nil {
			if name == "" {
				name = tc.Function.Name
			}
			// 兼容历史/第三方适配器：arguments 可能只存在于 Function.Arguments 字符串中。
			if len(arguments) == 0 && tc.Function.Arguments != "" {
				arguments = DecodeToolCallArguments(json.RawMessage(tc.Function.Arguments), name)
			}
		}

		encodedArgs := "{}"
		if len(arguments) > 0 {
			// 优先发送结构化参数，确保请求体是合法 JSON 对象字符串。
			if data, err := json.Marshal(arguments); err == nil {
				encodedArgs = string(data)
			}
		}
		// 若未能得到结构化参数，则保留原始字符串，避免信息损失。
		if tc.Function != nil && tc.Function.Arguments != "" && len(arguments) == 0 {
			encodedArgs = tc.Function.Arguments
		}

		out = append(out, ToolCall{
			ID:   tc.ID,
			Type: defaultString(tc.Type, "function"),
			Function: &FunctionCall{
				Name:      name,
				Arguments: encodedArgs,
			},
		})
	}
	return out
}

// ParseResponse 解析聊天补全响应，抽取首个 choice 并归一化 tool_calls。
func ParseResponse(body io.Reader) (*LLMResponse, error) {
	var apiResponse struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function *struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *UsageInfo `json:"usage"`
	}

	if err := json.NewDecoder(body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(apiResponse.Choices) == 0 {
		// 与多数 provider 行为对齐：无候选时视为正常结束，避免上层误判为异常。
		return &LLMResponse{
			FinishReason: "stop",
		}, nil
	}

	choice := apiResponse.Choices[0]
	toolCalls := make([]ToolCall, 0, len(choice.Message.ToolCalls))
	for _, tc := range choice.Message.ToolCalls {
		name := ""
		args := map[string]any{}
		rawArgs := ""
		if tc.Function != nil {
			name = tc.Function.Name
			// arguments 可能是对象或字符串化 JSON，这里统一解码为 map 便于上层处理。
			args = DecodeToolCallArguments(tc.Function.Arguments, name)
			// 同时保留原始 arguments 字符串，便于调试、回放或下游透传。
			rawArgsBytes, _ := json.Marshal(tc.Function.Arguments)
			rawArgs = string(rawArgsBytes)
			// 对非空原文优先使用原文，避免二次编码带来的引号/转义差异。
			if s := strings.TrimSpace(string(tc.Function.Arguments)); s != "" {
				rawArgs = s
			}
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:        tc.ID,
			Type:      tc.Type,
			Name:      name,
			Arguments: args,
			Function: &FunctionCall{
				Name:      name,
				Arguments: rawArgs,
			},
		})
	}

	return &LLMResponse{
		Content:          choice.Message.Content,
		ReasoningContent: choice.Message.ReasoningContent,
		ToolCalls:        toolCalls,
		FinishReason:     choice.FinishReason,
		Usage:            apiResponse.Usage,
	}, nil
}

// DecodeToolCallArguments 兼容多种参数编码：对象、字符串化 JSON、空值或异常载荷。
func DecodeToolCallArguments(raw json.RawMessage, name string) map[string]any {
	arguments := make(map[string]any)
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		// 空参数按空对象处理，减少上游判空分支。
		return arguments
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		log.Printf("providers/common: failed to decode tool call arguments payload for %q: %v", name, err)
		arguments["raw"] = string(raw)
		return arguments
	}

	switch v := decoded.(type) {
	case string:
		// 某些模型返回的是字符串化 JSON，需要二次反序列化。
		if strings.TrimSpace(v) == "" {
			return arguments
		}
		if err := json.Unmarshal([]byte(v), &arguments); err != nil {
			log.Printf("providers/common: failed to decode tool call arguments for %q: %v", name, err)
			arguments["raw"] = v
		}
		return arguments
	case map[string]any:
		// 标准对象参数，直接返回。
		return v
	default:
		// 兜底保留原始内容，避免参数丢失导致定位困难。
		log.Printf("providers/common: unsupported tool call arguments type for %q: %T", name, decoded)
		arguments["raw"] = string(raw)
		return arguments
	}
}

// HandleErrorResponse 读取错误响应体并识别 HTML 误配场景，输出可定位的摘要错误。
func HandleErrorResponse(resp *http.Response, apiBase string) error {
	contentType := resp.Header.Get("Content-Type")
	// 限制读取长度，避免错误页过大导致额外内存开销。
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 256))
	if readErr != nil {
		return fmt.Errorf("failed to read response: %w", readErr)
	}
	if LooksLikeHTML(body, contentType) {
		// 常见于 api_base 指向网页、反向代理拦截、鉴权网关重定向等场景。
		return WrapHTMLResponseError(resp.StatusCode, body, contentType, apiBase)
	}
	return fmt.Errorf(
		"API request failed:\n  Status: %d\n  Body:   %s",
		resp.StatusCode,
		ResponsePreview(body, 128),
	)
}

// ReadAndParseResponse 先探测响应前缀是否为 HTML，再按 JSON 解析，避免把网关页面当作模型响应。
func ReadAndParseResponse(resp *http.Response, apiBase string) (*LLMResponse, error) {
	contentType := resp.Header.Get("Content-Type")
	reader := bufio.NewReader(resp.Body)
	// 仅窥探前缀，不消费流内容，后续可继续完整 JSON 解码。
	prefix, err := reader.Peek(256)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return nil, fmt.Errorf("failed to inspect response: %w", err)
	}
	if LooksLikeHTML(prefix, contentType) {
		return nil, WrapHTMLResponseError(resp.StatusCode, prefix, contentType, apiBase)
	}
	out, err := ParseResponse(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}
	return out, nil
}

// LooksLikeHTML 基于 Content-Type 与正文前缀双重判断响应是否为 HTML。
func LooksLikeHTML(body []byte, contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	// 先信任明确的 MIME 类型声明。
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml+xml") {
		return true
	}
	// 再用正文前缀兜底识别，覆盖 MIME 错配场景。
	prefix := bytes.ToLower(leadingTrimmedPrefix(body, 128))
	return bytes.HasPrefix(prefix, []byte("<!doctype html")) ||
		bytes.HasPrefix(prefix, []byte("<html")) ||
		bytes.HasPrefix(prefix, []byte("<head")) ||
		bytes.HasPrefix(prefix, []byte("<body"))
}

// WrapHTMLResponseError 构造统一的 HTML 误响应错误信息。
func WrapHTMLResponseError(statusCode int, body []byte, contentType, apiBase string) error {
	return fmt.Errorf(
		"API request failed: %s returned HTML instead of JSON (content-type: %s); check api_base or proxy configuration.\n  Status: %d\n  Body:   %s",
		apiBase,
		contentType,
		statusCode,
		ResponsePreview(body, 128),
	)
}

// ResponsePreview 返回去空白后的响应预览，并在超长时截断。
func ResponsePreview(body []byte, maxLen int) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "<empty>"
	}
	if len(trimmed) <= maxLen {
		return string(trimmed)
	}
	// 省略号只用于展示，不影响真实响应体处理逻辑。
	return string(trimmed[:maxLen]) + "..."
}

// AsInt 尝试将通用数值类型转换为 int。
func AsInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case float32:
		return int(val), true
	default:
		return 0, false
	}
}

// AsFloat 尝试将通用数值类型转换为 float64。
func AsFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

// defaultString 在值为空白时返回回退值。
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// leadingTrimmedPrefix 返回去掉前导空白后的前缀片段，用于轻量内容探测。
func leadingTrimmedPrefix(body []byte, maxLen int) []byte {
	i := 0
	for i < len(body) {
		switch body[i] {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			// 跳过所有常见空白字符，避免误判 HTML 前缀。
			i++
		default:
			end := i + maxLen
			if end > len(body) {
				end = len(body)
			}
			return body[i:end]
		}
	}
	return nil
}
