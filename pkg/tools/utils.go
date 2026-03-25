package tools

import (
	"encoding/json"

	"github.com/argobell/clawcord/pkg/providers"
)

// NormalizeToolCall 将 provider 返回的 tool_call 统一成运行时使用的结构。
// 有些 provider 只填充 Function.Name / Function.Arguments，因此这里需要补齐
// Name 和 Arguments，避免后续注册表查找与工具执行失败。
func NormalizeToolCall(tc providers.ToolCall) providers.ToolCall {
	name := tc.Name
	args := tc.Arguments
	if tc.Function != nil {
		if name == "" {
			name = tc.Function.Name
		}
		if len(args) == 0 && tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
	}

	return providers.ToolCall{
		ID:        tc.ID,
		Type:      tc.Type,
		Function:  tc.Function,
		Name:      name,
		Arguments: args,
	}
}
