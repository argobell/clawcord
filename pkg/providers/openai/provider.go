package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/argobell/clawcord/pkg/providers/common"
	"github.com/argobell/clawcord/pkg/providers/protocoltypes"
)

// =============================================================================
// 类型别名 - 从 protocoltypes 导入
// =============================================================================

type (
	ToolCall               = protocoltypes.ToolCall               // 工具调用
	FunctionCall           = protocoltypes.FunctionCall           // 函数调用
	LLMResponse            = protocoltypes.LLMResponse            // LLM 响应
	UsageInfo              = protocoltypes.UsageInfo              // 使用信息
	Message                = protocoltypes.Message                // 消息
	ToolDefinition         = protocoltypes.ToolDefinition         // 工具定义
	ToolFunctionDefinition = protocoltypes.ToolFunctionDefinition // 工具函数定义
	ReasoningDetail        = protocoltypes.ReasoningDetail        // 推理详情
)

// 需要去除模型名称前缀的提供商列表
var stripModelPrefixProviders = map[string]struct{}{
	"litellm":    {},
	"moonshot":   {},
	"nvidia":     {},
	"groq":       {},
	"ollama":     {},
	"deepseek":   {},
	"google":     {},
	"openrouter": {},
	"zhipu":      {},
	"mistral":    {},
	"vivgrid":    {},
	"minimax":    {},
	"novita":     {},
	"lmstudio":   {},
}

// Provider 实现了 OpenAI 兼容的 LLM 提供商接口
type Provider struct {
	apiKey         string         // API 密钥
	apiBase        string         // API 基础地址
	maxTokensField string         // max_tokens 字段名称（某些模型使用 max_completion_tokens）
	httpClient     *http.Client   // HTTP 客户端
	extraBody      map[string]any // 额外的请求体参数
}

// Option 是 Provider 的配置选项函数类型
type Option func(*Provider)

// WithMaxTokensField 设置 max_tokens 字段名称
func WithMaxTokensField(maxTokensField string) Option {
	return func(p *Provider) {
		p.maxTokensField = maxTokensField
	}
}

// WithRequestTimeout 设置请求超时时间
func WithRequestTimeout(timeout time.Duration) Option {
	return func(p *Provider) {
		if timeout > 0 {
			p.httpClient.Timeout = timeout
		}
	}
}

// WithExtraBody 设置额外的请求体参数
func WithExtraBody(extraBody map[string]any) Option {
	return func(p *Provider) {
		p.extraBody = extraBody
	}
}

// NewProvider 创建新的 OpenAI 兼容提供商实例
func NewProvider(apiKey, apiBase, proxy string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:     apiKey,
		apiBase:    strings.TrimRight(apiBase, "/"),
		httpClient: common.NewHTTPClient(proxy),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// NewProviderWithMaxTokensField 创建带有自定义 max_tokens 字段名称的提供商
func NewProviderWithMaxTokensField(apiKey, apiBase, proxy, maxTokensField string) *Provider {
	return NewProvider(apiKey, apiBase, proxy, WithMaxTokensField(maxTokensField))
}

// NewProviderWithMaxTokensFieldAndTimeout 创建带有自定义 max_tokens 字段和超时的提供商
func NewProviderWithMaxTokensFieldAndTimeout(
	apiKey, apiBase, proxy, maxTokensField string,
	requestTimeoutSeconds int,
) *Provider {
	return NewProvider(
		apiKey,
		apiBase,
		proxy,
		WithMaxTokensField(maxTokensField),
		WithRequestTimeout(time.Duration(requestTimeoutSeconds)*time.Second),
	)
}

// buildRequestBody 构建聊天完成请求的请求体
//
// 请求体格式示例:
//   {
//     "model": "gpt-4",
//     "messages": [{"role": "user", "content": "Hello"}],
//     "tools": [...],           // 可选
//     "tool_choice": "auto",    // 启用工具时自动添加
//     "max_tokens": 100,        // 可选，某些模型用 max_completion_tokens
//     "temperature": 0.7,       // 可选，Kimi K2 强制为 1.0
//     "stream": false,          // ChatStream 设为 true
//   }
//
func (p *Provider) buildRequestBody(
	messages []Message, tools []ToolDefinition, model string, options map[string]any,
) map[string]any {
	model = normalizeModel(model, p.apiBase)

	requestBody := map[string]any{
		"model":    model,
		"messages": common.SerializeMessages(messages),
	}

	// 处理工具调用和原生搜索
	// native_search 仅对特定主机（如 api.openai.com）有效
	nativeSearch, _ := options["native_search"].(bool)
	nativeSearch = nativeSearch && isNativeSearchHost(p.apiBase)
	if len(tools) > 0 || nativeSearch {
		requestBody["tools"] = buildToolsList(tools, nativeSearch)
		requestBody["tool_choice"] = "auto"
	}

	// 处理 max_tokens 参数
	// 注意：不同模型使用不同的字段名
	// - GLM、o1、gpt-5 系列使用 max_completion_tokens
	// - 其他模型使用传统的 max_tokens
	if maxTokens, ok := common.AsInt(options["max_tokens"]); ok {
		fieldName := p.maxTokensField
		if fieldName == "" {
			lowerModel := strings.ToLower(model)
			if strings.Contains(lowerModel, "glm") || strings.Contains(lowerModel, "o1") ||
				strings.Contains(lowerModel, "gpt-5") {
				fieldName = "max_completion_tokens"
			} else {
				fieldName = "max_tokens"
			}
		}
		requestBody[fieldName] = maxTokens
	}

	// 处理 temperature 参数
	// 注意：Kimi K2 模型强制使用 temperature=1.0，忽略用户设置
	if temperature, ok := common.AsFloat(options["temperature"]); ok {
		lowerModel := strings.ToLower(model)
		if strings.Contains(lowerModel, "kimi") && strings.Contains(lowerModel, "k2") {
			requestBody["temperature"] = 1.0
		} else {
			requestBody["temperature"] = temperature
		}
	}

	if cacheKey, ok := options["prompt_cache_key"].(string); ok && cacheKey != "" {
		if supportsPromptCacheKey(p.apiBase) {
			requestBody["prompt_cache_key"] = cacheKey
		}
	}

	for k, v := range p.extraBody {
		requestBody[k] = v
	}

	return requestBody
}

// Chat 发送非流式聊天请求并返回完整响应
//
// 返回示例:
//   - 普通文本响应:
//     &LLMResponse{
//         Content:      "你好！有什么可以帮助你的吗？",
//         ToolCalls:    nil,
//         FinishReason: "stop",
//         Usage:        &UsageInfo{PromptTokens: 10, CompletionTokens: 15},
//     }
//
//   - 工具调用响应:
//     &LLMResponse{
//         Content:      "",
//         ToolCalls:    []ToolCall{
//             {ID: "call_123", Name: "get_weather", Arguments: map[string]any{"city": "北京"}},
//         },
//         FinishReason: "tool_calls",
//     }
//
func (p *Provider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	if p.apiBase == "" {
		return nil, fmt.Errorf("API base not configured")
	}

	requestBody := p.buildRequestBody(messages, tools, model, options)

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, common.HandleErrorResponse(resp, p.apiBase)
	}

	return common.ReadAndParseResponse(resp, p.apiBase)
}

// ChatStream 发送流式聊天请求，通过回调函数返回增量内容
//
// 返回示例:
//   - onChunk 回调会被多次调用，每次传入累积的文本:
//     onChunk("你好") → onChunk("你好！") → onChunk("你好！有什么") → ...
//
//   - 最终返回的 LLMResponse 包含完整结果:
//     &LLMResponse{
//         Content:      "你好！有什么可以帮助你的吗？",
//         ToolCalls:    nil,
//         FinishReason: "stop",
//         Usage:        &UsageInfo{PromptTokens: 10, CompletionTokens: 15},
//     }
//
//   - 流式工具调用（参数分多次到达）:
//     &LLMResponse{
//         Content:      "",
//         ToolCalls:    []ToolCall{
//             {ID: "call_456", Name: "search", Arguments: map[string]any{"query": "golang tutorial"}},
//         },
//         FinishReason: "tool_calls",
//     }
//
func (p *Provider) ChatStream(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
	onChunk func(accumulated string),
) (*LLMResponse, error) {
	if p.apiBase == "" {
		return nil, fmt.Errorf("API base not configured")
	}

	requestBody := p.buildRequestBody(messages, tools, model, options)
	requestBody["stream"] = true

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	// 流式请求使用独立的 HTTP 客户端（避免超时干扰）
	streamClient := &http.Client{Transport: p.httpClient.Transport}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, common.HandleErrorResponse(resp, p.apiBase)
	}

	return parseStreamResponse(ctx, resp.Body, onChunk)
}

// parseStreamResponse 解析流式响应的 SSE 数据
//
// 流式格式说明:
//   每行以 "data: " 开头，例如:
//     data: {"choices":[{"delta":{"content":"你好"}}]}
//     data: {"choices":[{"delta":{"content":"！"}}]}
//     data: [DONE]
//
//   工具调用在流式响应中的格式:
//     data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_123"}]}}]}
//     data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"get_weather"}}]}}]}
//     data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\\"city\\":\\"北京\\"}"}}]}}]}
//
func parseStreamResponse(
	ctx context.Context,
	reader io.Reader,
	onChunk func(accumulated string),
) (*LLMResponse, error) {
	var textContent strings.Builder
	var finishReason string
	var usage *UsageInfo

	type toolAccum struct {
		id       string
		name     string
		argsJSON strings.Builder
	}
	activeTools := map[int]*toolAccum{}

	// 创建扫描器并设置缓冲区（最大 10MB，处理大响应）
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function *struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *UsageInfo `json:"usage"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // skip malformed chunks
		}

		if chunk.Usage != nil {
			usage = chunk.Usage
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		if choice.Delta.Content != "" {
			textContent.WriteString(choice.Delta.Content)
			if onChunk != nil {
				onChunk(textContent.String())
			}
		}

		// 累积工具调用数据
		// 流式响应中，工具调用的 ID、名称和参数可能分散在多个 chunk 中
		for _, tc := range choice.Delta.ToolCalls {
			acc, ok := activeTools[tc.Index]
			if !ok {
				acc = &toolAccum{}
				activeTools[tc.Index] = acc
			}
			if tc.ID != "" {
				acc.id = tc.ID
			}
			if tc.Function != nil {
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.argsJSON.WriteString(tc.Function.Arguments)
				}
			}
		}

		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("streaming read error: %w", err)
	}

	var toolCalls []ToolCall
	// 按索引顺序组装所有收集到的工具调用
	// 流式响应中工具调用的参数可能分多次传输，需要累积后统一解析
	for i := 0; i < len(activeTools); i++ {
		acc, ok := activeTools[i]
		if !ok {
			continue
		}
		args := make(map[string]any)
		raw := acc.argsJSON.String()
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &args); err != nil {
				log.Printf("openai_compat stream: failed to decode tool call arguments for %q: %v", acc.name, err)
				args["raw"] = raw
			}
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:        acc.id,
			Name:      acc.name,
			Arguments: args,
		})
	}

	if finishReason == "" {
		finishReason = "stop"
	}

	return &LLMResponse{
		Content:      textContent.String(),
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
		Usage:        usage,
	}, nil
}

// normalizeModel 标准化模型名称，去除特定提供商的前缀
// normalizeModel 标准化模型名称
// 某些提供商（如 groq、deepseek 等）需要模型名称去除前缀（如 "groq/llama3" → "llama3"）
// OpenRouter 需要保留完整名称，其他情况根据 stripModelPrefixProviders 列表判断
func normalizeModel(model, apiBase string) string {
	before, after, ok := strings.Cut(model, "/")
	if !ok {
		return model
	}

	if strings.Contains(strings.ToLower(apiBase), "openrouter.ai") {
		return model
	}

	prefix := strings.ToLower(before)
	if _, ok := stripModelPrefixProviders[prefix]; ok {
		return after
	}

	return model
}

// buildToolsList 构建工具列表，支持原生搜索功能
// buildToolsList 构建工具列表
// 当启用原生搜索时，过滤掉用户自定义的 web_search 工具（避免冲突），
// 并添加提供商原生的 web_search_preview 工具
func buildToolsList(tools []ToolDefinition, nativeSearch bool) []any {
	result := make([]any, 0, len(tools)+1)
	for _, t := range tools {
		if nativeSearch && strings.EqualFold(t.Function.Name, "web_search") {
			continue
		}
		result = append(result, t)
	}
	if nativeSearch {
		result = append(result, map[string]any{"type": "web_search_preview"})
	}
	return result
}

// SupportsNativeSearch 检查当前提供商是否支持原生搜索
func (p *Provider) SupportsNativeSearch() bool {
	return isNativeSearchHost(p.apiBase)
}

// isNativeSearchHost 检查 API 基础地址是否支持原生搜索
func isNativeSearchHost(apiBase string) bool {
	u, err := url.Parse(apiBase)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "api.openai.com"
}

// supportsPromptCacheKey 检查 API 基础地址是否支持提示缓存密钥
func supportsPromptCacheKey(apiBase string) bool {
	u, err := url.Parse(apiBase)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "api.openai.com"
}
