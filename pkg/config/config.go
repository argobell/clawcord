package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FlexibleStringSlice supports string and numeric JSON values and comma-separated text.
type FlexibleStringSlice []string

func (f *FlexibleStringSlice) UnmarshalJSON(data []byte) error {
	var ss []string
	if err := json.Unmarshal(data, &ss); err == nil {
		*f = ss
		return nil
	}

	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	result := make([]string, 0, len(raw))
	for _, v := range raw {
		switch val := v.(type) {
		case string:
			result = append(result, val)
		case float64:
			result = append(result, fmt.Sprintf("%.0f", val))
		default:
			result = append(result, fmt.Sprintf("%v", val))
		}
	}
	*f = result
	return nil
}

func (f *FlexibleStringSlice) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*f = nil
		return nil
	}

	s := strings.ReplaceAll(string(text), "，", ",")
	parts := strings.Split(s, ",")

	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	*f = result
	return nil
}

type Config struct {
	Agents    AgentsConfig   `json:"agents"`
	Channels  ChannelsConfig `json:"channels"`
	Session   SessionConfig  `json:"session,omitempty"`
	ModelList []ModelConfig  `json:"model_list,omitempty"`
}

type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
	List     []AgentConfig `json:"list,omitempty"`
}

type AgentDefaults struct {
	Workspace         string   `json:"workspace,omitempty"`
	ModelName         string   `json:"model_name,omitempty"`
	Model             string   `json:"model,omitempty"`
	MaxTokens         int      `json:"max_tokens,omitempty"`
	Temperature       *float64 `json:"temperature,omitempty"`
	MaxToolIterations int      `json:"max_tool_iterations,omitempty"`
}

type AgentConfig struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Model     string `json:"model,omitempty"`
}

type SessionConfig struct {
	DMScope       string              `json:"dm_scope,omitempty"`
	IdentityLinks map[string][]string `json:"identity_links,omitempty"`
}

type ChannelsConfig struct {
	Discord DiscordConfig `json:"discord,omitempty"`
}

// GroupTriggerConfig controls when the bot responds in group chats.
type GroupTriggerConfig struct {
	MentionOnly bool     `json:"mention_only,omitempty"`
	Prefixes    []string `json:"prefixes,omitempty"`
}

// TypingConfig controls whether a channel should emit typing indicators.
type TypingConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

// PlaceholderConfig controls placeholder message behavior.
type PlaceholderConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Text    string `json:"text,omitempty"`
}

type DiscordConfig struct {
	Enabled            bool                `json:"enabled,omitempty"`
	Token              string              `json:"token"`
	Proxy              string              `json:"proxy,omitempty"`
	AllowFrom          FlexibleStringSlice `json:"allow_from,omitempty"`
	MentionOnly        bool                `json:"mention_only,omitempty"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	Typing             TypingConfig        `json:"typing,omitempty"`
	Placeholder        PlaceholderConfig   `json:"placeholder,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id,omitempty"`
	MessageContent     bool                `json:"message_content,omitempty"`
}

// ModelConfig defines a user-facing model alias and the runtime model it resolves to.
type ModelConfig struct {
	ModelName      string `json:"model_name"`
	Model          string `json:"model"`
	APIBase        string `json:"api_base,omitempty"`
	APIKey         string `json:"api_key,omitempty"`
	Proxy          string `json:"proxy,omitempty"`
	RequestTimeout int    `json:"request_timeout,omitempty"`
}

// GetModelName prefers the new model_name field and falls back to the legacy model field.
func (d *AgentDefaults) GetModelName() string {
	if d.ModelName != "" {
		return d.ModelName
	}
	return d.Model
}

// Validate checks that the config contains the required fields.
func (c *ModelConfig) Validate() error {
	if c.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

// GetModelConfig returns the first model_list entry that matches the given alias.
func (c *Config) GetModelConfig(modelName string) (*ModelConfig, error) {
	for i := range c.ModelList {
		if c.ModelList[i].ModelName != modelName {
			continue
		}
		if err := c.ModelList[i].Validate(); err != nil {
			return nil, fmt.Errorf("model_list[%d]: %w", i, err)
		}
		return &c.ModelList[i], nil
	}
	return nil, fmt.Errorf("model %q not found in model_list", modelName)
}
