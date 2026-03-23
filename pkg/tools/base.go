package tools

import "context"

// Tool 是所有工具必须实现的接口。
// 它定义了工具的基本行为，包括名称、描述、参数和执行逻辑。
type Tool interface {
	// Name 返回工具的名称。
	Name() string

	// Description 返回工具的描述。
	Description() string

	// Parameters 返回工具的参数定义。
	Parameters() map[string]any

	// Execute 执行工具的主要逻辑。
	// ctx 是上下文对象，args 是传递给工具的参数。
	// 返回值是工具执行的结果。
	Execute(ctx context.Context, args map[string]any) *ToolResult
}

// --- 请求范围内的工具上下文（channel / chatID）---
//
// 通过 context.Value 传递，以确保并发工具调用各自接收
// 自己的不可变副本——单例工具实例上没有可变状态。
//
// 键是未导出的指针类型变量——保证无冲突，
// 并且只能通过下面的辅助函数访问。

type toolCtxKey struct{ name string }

var (
	// ctxKeyChannel 是上下文中存储 channel 的键。
	ctxKeyChannel = &toolCtxKey{"channel"}
	// ctxKeyChatID 是上下文中存储 chatID 的键。
	ctxKeyChatID = &toolCtxKey{"chatID"}
)

// WithToolContext 返回一个携带 channel 和 chatID 的子上下文。
//
// 示例：
//
//	ctx := context.Background()
//	ctx = WithToolContext(ctx, "discord", "12345")
func WithToolContext(ctx context.Context, channel, chatID string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyChannel, channel)
	ctx = context.WithValue(ctx, ctxKeyChatID, chatID)
	return ctx
}

// ToolChannel 从上下文中提取 channel，如果未设置则返回 ""。
//
// 示例：
//
//	channel := ToolChannel(ctx)
func ToolChannel(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyChannel).(string)
	return v
}

// ToolChatID 从上下文中提取 chatID，如果未设置则返回 ""。
//
// 示例：
//
//	chatID := ToolChatID(ctx)
func ToolChatID(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyChatID).(string)
	return v
}

// AsyncCallback 是异步工具用来通知完成的函数类型。
// 当异步工具完成其工作时，它会调用此回调函数并传递结果。
//
// ctx 参数允许在代理关闭时取消回调。
// result 参数包含工具的执行结果。
//
// 示例：
//
//	callback := func(ctx context.Context, result *ToolResult) {
//		log.Println("异步操作完成", result)
//	}
type AsyncCallback func(ctx context.Context, result *ToolResult)

// AsyncExecutor 是工具可以实现的可选接口，用于支持
// 异步执行并带有完成回调。
//
// AsyncExecutor 将回调作为 ExecuteAsync 的参数接收。这消除了
// 并发调用可能会覆盖共享工具实例上的回调的数据竞争。
//
// 适用于：
//   - 不应阻塞 Agent 循环的长时间运行操作
//   - 独立完成的子代理生成
//   - 需要稍后报告结果的后台任务
//
// 示例：
//
//	func (t *SpawnTool) ExecuteAsync(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult {
//	    go func() {
//	        result := t.runSubagent(ctx, args)
//	        if cb != nil { cb(ctx, result) }
//	    }()
//	    return AsyncResult("子代理已生成，将稍后报告")
//	}
type AsyncExecutor interface {
	Tool
	// ExecuteAsync 异步运行工具。
	// cb 是异步操作完成时调用的回调函数。
	ExecuteAsync(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult
}

// ToolToSchema 将工具转换为 provider 可消费的函数工具 schema。
//
// 示例：
//
//	schema := ToolToSchema(myTool)
//	log.Println(schema)
func ToolToSchema(tool Tool) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  tool.Parameters(),
		},
	}
}
