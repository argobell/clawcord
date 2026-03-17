package bus

// Peer identifies the routing peer for a message (direct, group, channel, etc.)
// Peer 指明消息的路由对端（直接、群组、频道等）
type Peer struct {
	Kind string `json:"kind"` // "direct" | "group" | "channel" | ""
	ID   string `json:"id"`
}

// SenderInfo provides structured sender identity information.
// SenderInfo 提供结构化的发送者身份信息
type SenderInfo struct {
	Platform    string `json:"platform,omitempty"`     // "discord"
	PlatformID  string `json:"platform_id,omitempty"`  // raw platform ID, e.g. "123456"
	CanonicalID string `json:"canonical_id,omitempty"` // "platform:id" format
	Username    string `json:"username,omitempty"`     // username (e.g. @alice)
	DisplayName string `json:"display_name,omitempty"` // display name
}

// InboundMessage represents a message received from a channel, containing sender and content information.
// InboundMessage 表示从频道接收的消息，包含发送者和内容信息
type InboundMessage struct {
	Channel    string            `json:"channel"`
	SenderID   string            `json:"sender_id"`
	Sender     SenderInfo        `json:"sender"`
	ChatID     string            `json:"chat_id"`
	Content    string            `json:"content"`
	Media      []string          `json:"media,omitempty"`
	Peer       Peer              `json:"peer"`                  // routing peer
	MessageID  string            `json:"message_id,omitempty"`  // platform message ID
	MediaScope string            `json:"media_scope,omitempty"` // media lifecycle scope
	SessionKey string            `json:"session_key"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// OutboundMessage represents a message to be sent to a channel, containing recipient and content information.
// OutboundMessage 表示要发送到频道的消息，包含接收者和内容信息
type OutboundMessage struct {
	Channel          string `json:"channel"`
	ChatID           string `json:"chat_id"`
	Content          string `json:"content"`
	ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
}

// MediaPart describes a single media attachment to send.
// MediaPart 描述要发送的单个媒体附件
type MediaPart struct {
	Type        string `json:"type"`                   // "image" | "audio" | "video" | "file"
	Ref         string `json:"ref"`                    // media store ref, e.g. "media://abc123"
	Caption     string `json:"caption,omitempty"`      // optional caption text
	Filename    string `json:"filename,omitempty"`     // original filename hint
	ContentType string `json:"content_type,omitempty"` // MIME type hint
}

// OutboundMediaMessage carries media attachments from Agent to channels via the bus.
// OutboundMediaMessage 在 Agent 和频道之间通过总线传递媒体附件
type OutboundMediaMessage struct {
	Channel string      `json:"channel"`
	ChatID  string      `json:"chat_id"`
	Parts   []MediaPart `json:"parts"`
}
