package session

import "github.com/argobell/clawcord/pkg/providers"

// SessionStore 定义了 agent loop 使用的持久化操作。
// SessionManager 与 JSONLBackend 都实现了该接口，
// 因此可以在不修改 agent loop 代码的前提下替换存储层。
//
// 写方法（Add*、Set*、Truncate*）采用 fire-and-forget 方式：
// 它们不返回错误。具体实现应在内部记录失败日志。该设计与
// agent loop 所依赖的原始 SessionManager 约定保持一致。
type SessionStore interface {
	// AddMessage 向会话追加一条简单的角色/内容消息。
	AddMessage(sessionKey, role, content string)
	// AddFullMessage 向会话追加一条完整消息（包含工具调用信息）。
	AddFullMessage(sessionKey string, msg providers.Message)
	// GetHistory 返回该会话的完整消息历史。
	GetHistory(key string) []providers.Message
	// GetSummary 返回会话摘要；若不存在则返回空字符串。
	GetSummary(key string) string
	// SetSummary 替换会话摘要。
	SetSummary(key, summary string)
	// SetHistory 替换完整消息历史。
	SetHistory(key string, history []providers.Message)
	// TruncateHistory 仅保留最近 keepLast 条消息。
	TruncateHistory(key string, keepLast int)
	// Save 将待持久化状态写入持久存储。
	Save(key string) error
	// Close 释放 store 持有的资源。
	Close() error
}
