package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/argobell/clawcord/pkg/providers"
	"github.com/argobell/clawcord/pkg/providers/common"
)

// Provider 是 OpenAI 兼容接口的 provider 实现。
type Provider struct {
	apiKey  string
	apiBase string
	// maxTokensField 用于兼容不同模型的最大输出 token 字段名。
	maxTokensField string
	httpClient     *http.Client
}

// Option 用于定制 Provider 初始化参数。
type Option func(*Provider)

// WithMaxTokensField 显式设置最大输出 token 字段名。
func WithMaxTokensField(maxTokensField string) Option {
	return func(p *Provider) {
		p.maxTokensField = maxTokensField
	}
}

// WithRequestTimeout 设置请求超时时间（仅当 timeout > 0 时生效）。
func WithRequestTimeout(timeout time.Duration) Option {
	return func(p *Provider) {
		if timeout > 0 {
			p.httpClient.Timeout = timeout
		}
	}
}

// NewProvider 创建一个 OpenAI provider。
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

// NewProviderWithMaxTokensField 创建 provider 并指定 max tokens 字段名。
func NewProviderWithMaxTokensField(apiKey, apiBase, proxy, maxTokensField string) *Provider {
	return NewProvider(apiKey, apiBase, proxy, WithMaxTokensField(maxTokensField))
}

// NewProviderWithMaxTokensFieldAndTimeout 创建 provider 并同时设置 max tokens 字段和超时。
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

// Chat 发起一次 Chat Completions 请求并返回统一响应结构。
func (p *Provider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	// 基础配置校验，避免发送无效请求。
	if strings.TrimSpace(p.apiBase) == "" {
		return nil, fmt.Errorf("API base not configured")
	}

	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("model is required")
	}

	// 对 provider/model 形式做归一化，得到最终下游可识别模型名。
	model = normalizeModel(model, p.apiBase)
	requestBody := map[string]any{
		"model":    model,
		"messages": common.SerializeMessages(messages),
	}
	// 仅在存在工具定义时开启自动工具选择。
	if len(tools) > 0 {
		requestBody["tools"] = tools
		requestBody["tool_choice"] = "auto"
	}

	// 兼容不同模型的最大 token 字段命名差异。
	if maxTokens, ok := common.AsInt(options["max_tokens"]); ok {
		fieldName := p.maxTokensField
		if fieldName == "" {
			lowerModel := strings.ToLower(model)
			if strings.Contains(lowerModel, "glm") || strings.Contains(lowerModel, "o1") || strings.Contains(lowerModel, "gpt-5") {
				fieldName = "max_completion_tokens"
			} else {
				fieldName = "max_tokens"
			}
		}
		requestBody[fieldName] = maxTokens
	}

	// Kimi k2 系列仅支持 temperature=1，其他模型透传配置值。
	if temperature, ok := common.AsFloat(options["temperature"]); ok {
		lowerModel := strings.ToLower(model)
		if strings.Contains(lowerModel, "kimi") && strings.Contains(lowerModel, "k2") {
			temperature = 1.0
		}
		requestBody["temperature"] = temperature
	}

	// 仅对明确支持的 OpenAI/Azure OpenAI 端点附加 prompt_cache_key。
	if promptCacheKey, ok := options["prompt_cache_key"].(string); ok && promptCacheKey != "" && supportsPromptCacheKey(p.apiBase) {
		requestBody["prompt_cache_key"] = promptCacheKey
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		// 仅在存在密钥时注入 Bearer 认证头。
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

	// 成功响应统一走 common 解析，保持 provider 间行为一致。
	return common.ReadAndParseResponse(resp, p.apiBase)
}

// normalizeModel 兼容 "provider/model" 形式输入，必要时提取真实模型名。
func normalizeModel(model, apiBase string) string {
	before, after, ok := strings.Cut(model, "/")
	if !ok {
		return model
	}
	if strings.Contains(strings.ToLower(apiBase), "openrouter.ai") {
		// OpenRouter 需要完整模型标识，不能裁剪 provider 前缀。
		return model
	}
	switch strings.ToLower(before) {
	case "litellm", "moonshot", "nvidia", "groq", "ollama", "deepseek", "google",
		"openrouter", "zhipu", "mistral", "vivgrid", "minimax":
		return after
	default:
		return model
	}
}

// supportsPromptCacheKey 判断当前 API 基址是否支持 prompt_cache_key 字段。
func supportsPromptCacheKey(apiBase string) bool {
	if strings.TrimSpace(apiBase) == "" {
		return false
	}
	u, err := url.Parse(apiBase)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "api.openai.com" || strings.HasSuffix(host, ".openai.azure.com")
}
