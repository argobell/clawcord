package discord

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/argobell/clawcord/pkg/bus"
	"github.com/argobell/clawcord/pkg/channels"
	"github.com/argobell/clawcord/pkg/config"
	"github.com/argobell/clawcord/pkg/logger"
	"github.com/bwmarrin/discordgo"
)

const sendTimeout = 10 * time.Second

// DiscordChannel 是基于 discordgo 的最小可用 Discord 适配器。
type DiscordChannel struct {
	*channels.BaseChannel
	session        *discordgo.Session
	config         config.DiscordConfig
	ctx            context.Context
	cancel         context.CancelFunc
	typingMu       sync.Mutex
	typingSessions map[string]*typingSession
	botUserID      string
	handlerRemove  func()
	userFn         func() (*discordgo.User, error)
	openFn         func() error
	closeFn        func() error
	addHandlerFn   func(handler any) func()
	typingFn       func(chatID string) error
}

// New 创建最小可用的 DiscordChannel。
func New(cfg config.DiscordConfig, messageBus *bus.MessageBus) (*DiscordChannel, error) {
	session, err := discordgo.New("Bot " + strings.TrimSpace(cfg.Token))
	if err != nil {
		return nil, fmt.Errorf("failed to create discord session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentGuildMessages | discordgo.IntentDirectMessages
	if cfg.MessageContent {
		session.Identify.Intents |= discordgo.IntentMessageContent
	}

	base := channels.NewBaseChannel(
		"discord",
		cfg,
		messageBus,
		[]string(cfg.AllowFrom),
		channels.WithMaxMessageLength(2000),
		channels.WithGroupTrigger(normalizeGroupTrigger(cfg)),
		channels.WithReasoningChannelID(cfg.ReasoningChannelID),
	)

	ch := &DiscordChannel{
		BaseChannel:    base,
		session:        session,
		config:         cfg,
		ctx:            context.Background(),
		typingSessions: make(map[string]*typingSession),
	}
	ch.userFn = func() (*discordgo.User, error) { return ch.session.User("@me") }
	ch.openFn = func() error { return ch.session.Open() }
	ch.closeFn = func() error { return ch.session.Close() }
	ch.addHandlerFn = func(handler any) func() { return ch.session.AddHandler(handler) }
	ch.typingFn = func(chatID string) error { return ch.session.ChannelTyping(chatID) }
	ch.SetOwner(ch)
	return ch, nil
}

// Start 连接 Discord 网关并开始接收入站消息。
func (c *DiscordChannel) Start(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}

	botUser, err := c.userFn()
	if err != nil {
		return fmt.Errorf("failed to get bot user: %w", err)
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.botUserID = botUser.ID
	if c.handlerRemove != nil {
		c.handlerRemove()
		c.handlerRemove = nil
	}
	c.handlerRemove = c.addHandlerFn(c.handleMessage)

	if err := c.openFn(); err != nil {
		if c.handlerRemove != nil {
			c.handlerRemove()
			c.handlerRemove = nil
		}
		return fmt.Errorf("failed to open discord session: %w", err)
	}

	c.SetRunning(true)
	return nil
}

// Stop 关闭网关连接并停止所有 typing 循环。
func (c *DiscordChannel) Stop(ctx context.Context) error {
	_ = ctx
	c.SetRunning(false)

	c.typingMu.Lock()
	for chatID, session := range c.typingSessions {
		close(session.stop)
		delete(c.typingSessions, chatID)
	}
	c.typingMu.Unlock()

	if c.cancel != nil {
		c.cancel()
	}
	if c.handlerRemove != nil {
		c.handlerRemove()
		c.handlerRemove = nil
	}

	if err := c.closeFn(); err != nil {
		return fmt.Errorf("failed to close discord session: %w", err)
	}
	return nil
}

// Send 发送一条最小文本消息。
func (c *DiscordChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("discord channel is not running")
	}
	if strings.TrimSpace(msg.ChatID) == "" {
		return fmt.Errorf("channel ID is empty")
	}
	if len([]rune(msg.Content)) == 0 {
		return nil
	}

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		var err error
		if msg.ReplyToMessageID != "" {
			_, err = c.session.ChannelMessageSendComplex(msg.ChatID, &discordgo.MessageSend{
				Content: msg.Content,
				Reference: &discordgo.MessageReference{
					MessageID: msg.ReplyToMessageID,
					ChannelID: msg.ChatID,
				},
			})
		} else {
			_, err = c.session.ChannelMessageSend(msg.ChatID, msg.Content)
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("discord send failed: %w", err)
		}
		return nil
	case <-sendCtx.Done():
		return sendCtx.Err()
	}
}

// EditMessage 编辑已经发出的消息。
func (c *DiscordChannel) EditMessage(ctx context.Context, chatID string, messageID string, content string) error {
	_ = ctx
	_, err := c.session.ChannelMessageEdit(chatID, messageID, content)
	return err
}

// SendPlaceholder 发送后续可编辑的占位消息。
func (c *DiscordChannel) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	_ = ctx
	if !c.config.Placeholder.Enabled {
		return "", nil
	}

	text := c.config.Placeholder.Text
	if text == "" {
		text = "Thinking... 💭"
	}

	msg, err := c.session.ChannelMessageSend(chatID, text)
	if err != nil {
		return "", err
	}
	return msg.ID, nil
}

// StartTyping 启动持续 typing 指示，并返回幂等停止函数。
func (c *DiscordChannel) StartTyping(ctx context.Context, chatID string) (func(), error) {
	_ = ctx
	return c.startTyping(chatID), nil
}

func (c *DiscordChannel) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m == nil || m.Message == nil || m.Author == nil {
		return
	}

	selfID := c.botUserID
	if selfID == "" && s != nil && s.State != nil && s.State.User != nil {
		selfID = s.State.User.ID
	}
	if selfID != "" && m.Author.ID == selfID {
		return
	}

	sender := bus.SenderInfo{
		Platform:    "discord",
		PlatformID:  m.Author.ID,
		CanonicalID: buildCanonicalID(m.Author.ID),
		Username:    m.Author.Username,
		DisplayName: buildDisplayName(m.Author),
	}

	if !c.IsAllowedSender(sender) {
		return
	}

	content := m.Content
	if m.GuildID != "" {
		isMentioned := false
		for _, mention := range m.Mentions {
			if mention != nil && mention.ID == selfID {
				isMentioned = true
				break
			}
		}
		content = c.stripBotMention(content)
		respond, cleaned := c.ShouldRespondInGroup(isMentioned, content)
		if !respond {
			return
		}
		content = cleaned
	} else {
		content = c.stripBotMention(content)
	}

	media := collectAttachmentURLs(m.Attachments)
	if content == "" && len(media) == 0 {
		return
	}
	if content == "" {
		content = "[media only]"
	}

	peerKind := "channel"
	peerID := m.ChannelID
	if m.GuildID == "" {
		peerKind = "direct"
		peerID = m.Author.ID
	}

	metadata := map[string]string{
		"user_id":      m.Author.ID,
		"username":     m.Author.Username,
		"display_name": sender.DisplayName,
		"guild_id":     m.GuildID,
		"channel_id":   m.ChannelID,
		"is_dm":        fmt.Sprintf("%t", m.GuildID == ""),
	}

	c.HandleMessage(
		c.ctx,
		bus.Peer{Kind: peerKind, ID: peerID},
		m.ID,
		m.Author.ID,
		m.ChannelID,
		content,
		media,
		metadata,
		sender,
	)
}

type typingSession struct {
	stop chan struct{}
	refs int
}

func (c *DiscordChannel) startTyping(chatID string) func() {
	c.typingMu.Lock()
	if existing, ok := c.typingSessions[chatID]; ok {
		existing.refs++
		c.typingMu.Unlock()
		return func() { c.stopTyping(chatID, existing) }
	}

	session := &typingSession{
		stop: make(chan struct{}),
		refs: 1,
	}
	c.typingSessions[chatID] = session
	c.typingMu.Unlock()

	go func() {
		if err := c.typingFn(chatID); err != nil {
			logger.DebugCF("discord", "ChannelTyping error", map[string]any{"chat_id": chatID, "error": err.Error()})
		}

		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		timeout := time.After(5 * time.Minute)

		for {
			select {
			case <-session.stop:
				return
			case <-timeout:
				return
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				if err := c.typingFn(chatID); err != nil {
					logger.DebugCF("discord", "ChannelTyping error", map[string]any{"chat_id": chatID, "error": err.Error()})
				}
			}
		}
	}()

	return func() { c.stopTyping(chatID, session) }
}

func (c *DiscordChannel) stopTyping(chatID string, session *typingSession) {
	c.typingMu.Lock()
	defer c.typingMu.Unlock()

	current, ok := c.typingSessions[chatID]
	if !ok || current != session {
		return
	}

	current.refs--
	if current.refs > 0 {
		return
	}
	close(current.stop)
	delete(c.typingSessions, chatID)
}

func (c *DiscordChannel) stripBotMention(text string) string {
	if c.botUserID == "" {
		return strings.TrimSpace(text)
	}

	text = strings.ReplaceAll(text, fmt.Sprintf("<@%s>", c.botUserID), "")
	text = strings.ReplaceAll(text, fmt.Sprintf("<@!%s>", c.botUserID), "")
	return strings.TrimSpace(text)
}

func buildCanonicalID(platformID string) string {
	if strings.TrimSpace(platformID) == "" {
		return ""
	}
	return "discord:" + platformID
}

func buildDisplayName(user *discordgo.User) string {
	if user == nil {
		return ""
	}
	if user.Discriminator != "" && user.Discriminator != "0" {
		return user.Username + "#" + user.Discriminator
	}
	return user.Username
}

func collectAttachmentURLs(attachments []*discordgo.MessageAttachment) []string {
	if len(attachments) == 0 {
		return nil
	}

	media := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil || strings.TrimSpace(attachment.URL) == "" {
			continue
		}
		media = append(media, attachment.URL)
	}
	return media
}

func normalizeGroupTrigger(cfg config.DiscordConfig) config.GroupTriggerConfig {
	if cfg.MentionOnly && !cfg.GroupTrigger.MentionOnly {
		cfg.GroupTrigger.MentionOnly = true
	}
	return cfg.GroupTrigger
}
