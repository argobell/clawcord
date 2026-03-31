package discord

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/argobell/clawcord/pkg/bus"
	"github.com/argobell/clawcord/pkg/channels"
	"github.com/argobell/clawcord/pkg/config"
	"github.com/argobell/clawcord/pkg/logger"
	"github.com/argobell/clawcord/pkg/media"
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
	sendComplexFn  func(channelID string, msg *discordgo.MessageSend) (*discordgo.Message, error)
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
	ch.sendComplexFn = func(channelID string, msg *discordgo.MessageSend) (*discordgo.Message, error) {
		return ch.session.ChannelMessageSendComplex(channelID, msg)
	}
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
			_, err = c.sendComplexFn(msg.ChatID, &discordgo.MessageSend{
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

// SendMedia sends media attachments resolved from the configured media store.
func (c *DiscordChannel) SendMedia(ctx context.Context, msg bus.OutboundMediaMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("discord channel is not running")
	}
	if strings.TrimSpace(msg.ChatID) == "" {
		return fmt.Errorf("channel ID is empty")
	}

	store := c.GetMediaStore()
	if store == nil {
		return fmt.Errorf("no media store configured")
	}

	files := make([]*discordgo.File, 0, len(msg.Parts))
	var caption string
	for _, part := range msg.Parts {
		localPath, err := store.Resolve(part.Ref)
		if err != nil {
			logger.ErrorCF("discord", "Failed to resolve media ref", map[string]any{
				"ref":   part.Ref,
				"error": err.Error(),
			})
			continue
		}

		file, err := os.Open(localPath)
		if err != nil {
			logger.ErrorCF("discord", "Failed to open media file", map[string]any{
				"path":  localPath,
				"error": err.Error(),
			})
			continue
		}

		filename := part.Filename
		if filename == "" {
			filename = "file"
		}
		files = append(files, &discordgo.File{
			Name:        filename,
			ContentType: part.ContentType,
			Reader:      file,
		})
		if caption == "" && part.Caption != "" {
			caption = part.Caption
		}
	}
	if len(files) == 0 {
		return nil
	}

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.sendComplexFn(msg.ChatID, &discordgo.MessageSend{
			Content: caption,
			Files:   files,
		})
		done <- err
	}()

	closeFiles := func() {
		for _, file := range files {
			if closer, ok := file.Reader.(*os.File); ok {
				_ = closer.Close()
			}
		}
	}

	select {
	case err := <-done:
		closeFiles()
		if err != nil {
			return fmt.Errorf("discord send media failed: %w", err)
		}
		return nil
	case <-sendCtx.Done():
		closeFiles()
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

	media := c.collectInboundMedia(channels.BuildMediaScope("discord", m.ChannelID, m.ID), m.Attachments)
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

func (c *DiscordChannel) collectInboundMedia(scope string, attachments []*discordgo.MessageAttachment) []string {
	if len(attachments) == 0 {
		return nil
	}

	store := c.GetMediaStore()
	mediaRefs := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil || strings.TrimSpace(attachment.URL) == "" {
			continue
		}
		if store == nil || !shouldStoreAttachment(attachment) {
			mediaRefs = append(mediaRefs, attachment.URL)
			continue
		}

		localPath, contentType, err := downloadAttachment(attachment.URL, attachment.Filename)
		if err != nil {
			logger.WarnCF("discord", "Failed to download inbound attachment", map[string]any{
				"url":      attachment.URL,
				"filename": attachment.Filename,
				"error":    err.Error(),
			})
			mediaRefs = append(mediaRefs, attachment.URL)
			continue
		}

		ref, err := store.Store(localPath, media.MediaMeta{
			Filename:    attachment.Filename,
			ContentType: firstNonEmpty(strings.TrimSpace(attachment.ContentType), contentType),
			Source:      "discord",
		}, scope)
		if err != nil {
			_ = os.Remove(localPath)
			logger.WarnCF("discord", "Failed to store inbound attachment", map[string]any{
				"url":      attachment.URL,
				"filename": attachment.Filename,
				"error":    err.Error(),
			})
			mediaRefs = append(mediaRefs, attachment.URL)
			continue
		}

		mediaRefs = append(mediaRefs, ref)
	}
	return mediaRefs
}

func shouldStoreAttachment(attachment *discordgo.MessageAttachment) bool {
	if attachment == nil {
		return false
	}

	contentType := strings.ToLower(strings.TrimSpace(attachment.ContentType))
	if strings.HasPrefix(contentType, "image/") {
		return true
	}

	switch strings.ToLower(filepath.Ext(strings.TrimSpace(attachment.Filename))) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

func downloadAttachment(rawURL, filename string) (string, string, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	pattern := "clawcord-discord-*"
	if ext := filepath.Ext(strings.TrimSpace(filename)); ext != "" {
		pattern += ext
	}

	tmp, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", "", err
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", "", err
	}

	return tmp.Name(), strings.TrimSpace(resp.Header.Get("Content-Type")), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeGroupTrigger(cfg config.DiscordConfig) config.GroupTriggerConfig {
	if cfg.MentionOnly && !cfg.GroupTrigger.MentionOnly {
		cfg.GroupTrigger.MentionOnly = true
	}
	return cfg.GroupTrigger
}
