package channels

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/argobell/clawcord/pkg/bus"
	"github.com/argobell/clawcord/pkg/config"
	"github.com/argobell/clawcord/pkg/logger"
	"github.com/argobell/clawcord/pkg/media"
)

var (
	uniqueIDCounter uint64
	uniqueIDPrefix  string
)

func init() {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		binary.BigEndian.PutUint64(b[:], uint64(time.Now().UnixNano()))
	}
	uniqueIDPrefix = hex.EncodeToString(b[:])
}

// Channel 定义所有频道适配器共享的最小运行时契约。
type Channel interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, msg bus.OutboundMessage) error
	IsRunning() bool
	IsAllowed(senderID string) bool
	IsAllowedSender(sender bus.SenderInfo) bool
	ReasoningChannelID() string
}

// BaseChannelOption 用于配置 BaseChannel。
type BaseChannelOption func(*BaseChannel)

// WithMaxMessageLength 设置频道允许的最大消息长度（按 rune 计）。
func WithMaxMessageLength(n int) BaseChannelOption {
	return func(c *BaseChannel) { c.maxMessageLength = n }
}

// WithGroupTrigger 设置群聊触发规则。
func WithGroupTrigger(gt config.GroupTriggerConfig) BaseChannelOption {
	return func(c *BaseChannel) { c.groupTrigger = gt }
}

// WithReasoningChannelID 设置推理输出频道 ID。
func WithReasoningChannelID(id string) BaseChannelOption {
	return func(c *BaseChannel) { c.reasoningChannelID = id }
}

// MessageLengthProvider 让 manager 以后可以通过类型断言读取消息长度限制。
type MessageLengthProvider interface {
	MaxMessageLength() int
}

// BaseChannel 承载所有频道共享的入站规范化行为。
type BaseChannel struct {
	config              any
	bus                 *bus.MessageBus
	running             atomic.Bool
	name                string
	allowList           []string
	maxMessageLength    int
	groupTrigger        config.GroupTriggerConfig
	placeholderRecorder PlaceholderRecorder
	owner               Channel
	reasoningChannelID  string
	mediaStore          media.MediaStore
}

func NewBaseChannel(
	name string,
	config any,
	messageBus *bus.MessageBus,
	allowList []string,
	opts ...BaseChannelOption,
) *BaseChannel {
	bc := &BaseChannel{
		config:    config,
		bus:       messageBus,
		name:      name,
		allowList: allowList,
	}
	for _, opt := range opts {
		opt(bc)
	}
	return bc
}

func (c *BaseChannel) MaxMessageLength() int {
	return c.maxMessageLength
}

// ShouldRespondInGroup 复用 picoclaw 的群聊触发语义。
func (c *BaseChannel) ShouldRespondInGroup(isMentioned bool, content string) (bool, string) {
	gt := c.groupTrigger

	if isMentioned {
		return true, strings.TrimSpace(content)
	}

	if gt.MentionOnly {
		return false, content
	}

	if len(gt.Prefixes) > 0 {
		for _, prefix := range gt.Prefixes {
			if prefix != "" && strings.HasPrefix(content, prefix) {
				return true, strings.TrimSpace(strings.TrimPrefix(content, prefix))
			}
		}
		return false, content
	}

	return true, strings.TrimSpace(content)
}

func (c *BaseChannel) Name() string {
	return c.name
}

func (c *BaseChannel) ReasoningChannelID() string {
	return c.reasoningChannelID
}

func (c *BaseChannel) IsRunning() bool {
	return c.running.Load()
}

func (c *BaseChannel) IsAllowed(senderID string) bool {
	if len(c.allowList) == 0 {
		return true
	}

	idPart := senderID
	userPart := ""
	if idx := strings.Index(senderID, "|"); idx > 0 {
		idPart = senderID[:idx]
		userPart = senderID[idx+1:]
	}

	for _, allowed := range c.allowList {
		trimmed := strings.TrimPrefix(allowed, "@")
		allowedID := trimmed
		allowedUser := ""
		if idx := strings.Index(trimmed, "|"); idx > 0 {
			allowedID = trimmed[:idx]
			allowedUser = trimmed[idx+1:]
		}

		if senderID == allowed ||
			idPart == allowed ||
			senderID == trimmed ||
			idPart == trimmed ||
			idPart == allowedID ||
			(allowedUser != "" && senderID == allowedUser) ||
			(userPart != "" && (userPart == allowed || userPart == trimmed || userPart == allowedUser)) {
			return true
		}
	}

	return false
}

func (c *BaseChannel) IsAllowedSender(sender bus.SenderInfo) bool {
	if len(c.allowList) == 0 {
		return true
	}

	for _, allowed := range c.allowList {
		if matchAllowedSender(sender, allowed) {
			return true
		}
	}

	return false
}

// HandleMessage 将平台事件规范化为总线上的 InboundMessage。
func (c *BaseChannel) HandleMessage(
	ctx context.Context,
	peer bus.Peer,
	messageID, senderID, chatID, content string,
	media []string,
	metadata map[string]string,
	senderOpts ...bus.SenderInfo,
) {
	var sender bus.SenderInfo
	if len(senderOpts) > 0 {
		sender = senderOpts[0]
	}

	if sender.CanonicalID != "" || sender.PlatformID != "" || sender.Username != "" {
		if !c.IsAllowedSender(sender) {
			return
		}
	} else if !c.IsAllowed(senderID) {
		return
	}

	resolvedSenderID := senderID
	if sender.CanonicalID != "" {
		resolvedSenderID = sender.CanonicalID
	}

	msg := bus.InboundMessage{
		Channel:    c.name,
		SenderID:   resolvedSenderID,
		Sender:     sender,
		ChatID:     chatID,
		Content:    content,
		Media:      media,
		Peer:       peer,
		MessageID:  messageID,
		MediaScope: BuildMediaScope(c.name, chatID, messageID),
		SessionKey: BuildSessionKey(c.name, peer, chatID),
		Metadata:   metadata,
	}

	if c.owner != nil && c.placeholderRecorder != nil {
		if tc, ok := c.owner.(TypingCapable); ok {
			if stop, err := tc.StartTyping(ctx, chatID); err == nil && stop != nil {
				c.placeholderRecorder.RecordTypingStop(c.name, chatID, messageID, stop)
			}
		}
		if pc, ok := c.owner.(PlaceholderCapable); ok {
			if phID, err := pc.SendPlaceholder(ctx, chatID); err == nil && phID != "" {
				c.placeholderRecorder.RecordPlaceholder(c.name, chatID, messageID, phID)
			}
		}
	}

	if c.bus == nil {
		return
	}
	if err := c.bus.PublishInbound(ctx, msg); err != nil {
		logger.ErrorCF("channels", "Failed to publish inbound message", map[string]any{
			"channel": c.name,
			"chat_id": chatID,
			"error":   err.Error(),
		})
	}
}

func (c *BaseChannel) SetRunning(running bool) {
	c.running.Store(running)
}

func (c *BaseChannel) SetPlaceholderRecorder(r PlaceholderRecorder) {
	c.placeholderRecorder = r
}

func (c *BaseChannel) GetPlaceholderRecorder() PlaceholderRecorder {
	return c.placeholderRecorder
}

func (c *BaseChannel) SetOwner(ch Channel) {
	c.owner = ch
}

func (c *BaseChannel) SetMediaStore(s media.MediaStore) {
	c.mediaStore = s
}

func (c *BaseChannel) GetMediaStore() media.MediaStore {
	return c.mediaStore
}

func BuildMediaScope(channel, chatID, messageID string) string {
	id := messageID
	if id == "" {
		id = uniqueID()
	}
	return channel + ":" + chatID + ":" + id
}

func BuildSessionKey(channel string, peer bus.Peer, chatID string) string {
	kind := strings.TrimSpace(peer.Kind)
	if kind == "" {
		kind = "direct"
	}

	id := strings.TrimSpace(peer.ID)
	if id == "" {
		id = strings.TrimSpace(chatID)
	}
	if id == "" {
		id = "unknown"
	}

	return channel + ":" + kind + ":" + id
}

func uniqueID() string {
	n := atomic.AddUint64(&uniqueIDCounter, 1)
	return uniqueIDPrefix + strconv.FormatUint(n, 16)
}

func matchAllowedSender(sender bus.SenderInfo, allowed string) bool {
	allowed = strings.TrimSpace(allowed)
	if allowed == "" {
		return false
	}

	if strings.HasPrefix(allowed, "@") {
		return strings.EqualFold(sender.Username, strings.TrimPrefix(allowed, "@"))
	}

	if sender.CanonicalID != "" && strings.EqualFold(sender.CanonicalID, allowed) {
		return true
	}

	allowedID := allowed
	allowedUser := ""
	if idx := strings.Index(allowed, "|"); idx > 0 {
		allowedID = allowed[:idx]
		allowedUser = allowed[idx+1:]
	}

	if sender.PlatformID != "" && strings.EqualFold(sender.PlatformID, allowedID) {
		return true
	}
	if sender.CanonicalID != "" && strings.EqualFold(sender.CanonicalID, allowedID) {
		return true
	}
	if allowedUser != "" && strings.EqualFold(sender.Username, allowedUser) {
		return true
	}
	if sender.Username != "" && strings.EqualFold(sender.Username, allowedID) {
		return true
	}

	return false
}
