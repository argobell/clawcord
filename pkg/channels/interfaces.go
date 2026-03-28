package channels

import "context"

// TypingCapable 表示频道支持显示输入中/思考中的提示。
type TypingCapable interface {
	StartTyping(ctx context.Context, chatID string) (stop func(), err error)
}

// MessageEditor 表示频道支持编辑已经发送的消息。
type MessageEditor interface {
	EditMessage(ctx context.Context, chatID string, messageID string, content string) error
}

// PlaceholderCapable 表示频道支持先发送占位消息，后续再编辑为真实回复。
type PlaceholderCapable interface {
	SendPlaceholder(ctx context.Context, chatID string) (messageID string, err error)
}

// PlaceholderRecorder 由后续的 manager 注入，记录占位消息和 typing 状态。
type PlaceholderRecorder interface {
	RecordPlaceholder(channel, chatID, messageID, placeholderID string)
	RecordTypingStop(channel, chatID, messageID string, stop func())
}
