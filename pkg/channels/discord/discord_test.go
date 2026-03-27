package discord

import (
	"context"
	"testing"

	"github.com/argobell/clawcord/pkg/bus"
	projectconfig "github.com/argobell/clawcord/pkg/config"
	"github.com/bwmarrin/discordgo"
)

func TestNewDiscordChannel(t *testing.T) {
	messageBus := bus.NewMessageBus()

	ch, err := New(projectconfig.DiscordConfig{
		Token:     "test-token",
		AllowFrom: projectconfig.FlexibleStringSlice{"discord:123"},
	}, messageBus)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if ch.Name() != "discord" {
		t.Fatalf("Name() = %q, want %q", ch.Name(), "discord")
	}
	if ch.MaxMessageLength() != 2000 {
		t.Fatalf("MaxMessageLength() = %d, want 2000", ch.MaxMessageLength())
	}
	if ch.session == nil {
		t.Fatal("session is nil")
	}
	if ch.ctx == nil {
		t.Fatal("ctx is nil")
	}
}

func TestNewDiscordChannelSetsGatewayIntents(t *testing.T) {
	messageBus := bus.NewMessageBus()

	ch, err := New(projectconfig.DiscordConfig{
		Token:          "test-token",
		MessageContent: true,
	}, messageBus)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	intents := ch.session.Identify.Intents
	if intents&discordgo.IntentGuildMessages == 0 {
		t.Fatal("IntentGuildMessages is not enabled")
	}
	if intents&discordgo.IntentDirectMessages == 0 {
		t.Fatal("IntentDirectMessages is not enabled")
	}
	if intents&discordgo.IntentMessageContent == 0 {
		t.Fatal("IntentMessageContent is not enabled")
	}
}

func TestNewDiscordChannelDoesNotEnableMessageContentWhenDisabled(t *testing.T) {
	messageBus := bus.NewMessageBus()

	ch, err := New(projectconfig.DiscordConfig{
		Token:          "test-token",
		MessageContent: false,
	}, messageBus)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if ch.session.Identify.Intents&discordgo.IntentMessageContent != 0 {
		t.Fatal("IntentMessageContent should not be enabled")
	}
}

func TestHandleMessagePublishesDirectMessage(t *testing.T) {
	messageBus := bus.NewMessageBus()

	ch, err := New(projectconfig.DiscordConfig{Token: "test-token"}, messageBus)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ch.botUserID = "bot-user"

	session := mustNewSession(t)
	session.State.User = &discordgo.User{ID: "bot-user"}

	ch.handleMessage(session, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-1",
			ChannelID: "dm-1",
			Content:   "hello",
			Author: &discordgo.User{
				ID:            "123456",
				Username:      "alice",
				Discriminator: "1234",
			},
		},
	})

	msg, ok := messageBus.ConsumeInbound(context.Background())
	if !ok {
		t.Fatal("expected inbound message")
	}

	if msg.Channel != "discord" {
		t.Fatalf("Channel = %q, want %q", msg.Channel, "discord")
	}
	if msg.SenderID != "discord:123456" {
		t.Fatalf("SenderID = %q, want %q", msg.SenderID, "discord:123456")
	}
	if msg.ChatID != "dm-1" {
		t.Fatalf("ChatID = %q, want %q", msg.ChatID, "dm-1")
	}
	if msg.Content != "hello" {
		t.Fatalf("Content = %q, want %q", msg.Content, "hello")
	}
	if msg.Peer.Kind != "direct" || msg.Peer.ID != "123456" {
		t.Fatalf("Peer = %+v, want direct/123456", msg.Peer)
	}
	if msg.SessionKey != "discord:direct:123456" {
		t.Fatalf("SessionKey = %q, want %q", msg.SessionKey, "discord:direct:123456")
	}
	if msg.Sender.Username != "alice" {
		t.Fatalf("Sender.Username = %q, want %q", msg.Sender.Username, "alice")
	}
}

func TestHandleMessageIgnoresUnmentionedGuildMessageWhenMentionOnly(t *testing.T) {
	messageBus := bus.NewMessageBus()

	ch, err := New(projectconfig.DiscordConfig{
		Token: "test-token",
		GroupTrigger: projectconfig.GroupTriggerConfig{
			MentionOnly: true,
		},
	}, messageBus)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ch.botUserID = "bot-user"

	session := mustNewSession(t)
	session.State.User = &discordgo.User{ID: "bot-user"}

	ch.handleMessage(session, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-1",
			ChannelID: "guild-channel-1",
			GuildID:   "guild-1",
			Content:   "hello there",
			Author: &discordgo.User{
				ID:       "123456",
				Username: "alice",
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, ok := messageBus.ConsumeInbound(ctx); ok {
		t.Fatal("expected no inbound message")
	}
}

func TestHandleMessagePublishesMediaOnlyMessage(t *testing.T) {
	messageBus := bus.NewMessageBus()

	ch, err := New(projectconfig.DiscordConfig{Token: "test-token"}, messageBus)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ch.botUserID = "bot-user"

	session := mustNewSession(t)
	session.State.User = &discordgo.User{ID: "bot-user"}

	ch.handleMessage(session, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-1",
			ChannelID: "dm-1",
			Content:   "",
			Author: &discordgo.User{
				ID:       "123456",
				Username: "alice",
			},
			Attachments: []*discordgo.MessageAttachment{
				{
					ID:       "att-1",
					Filename: "image.png",
					URL:      "https://cdn.discordapp.com/test/image.png",
				},
			},
		},
	})

	msg, ok := messageBus.ConsumeInbound(context.Background())
	if !ok {
		t.Fatal("expected inbound message")
	}

	if msg.Content != "[media only]" {
		t.Fatalf("Content = %q, want %q", msg.Content, "[media only]")
	}
	if len(msg.Media) != 1 || msg.Media[0] != "https://cdn.discordapp.com/test/image.png" {
		t.Fatalf("Media = %#v, want attachment url", msg.Media)
	}
}

func TestStartStopManagesHandlerLifecycle(t *testing.T) {
	ch, err := New(projectconfig.DiscordConfig{Token: "test-token"}, bus.NewMessageBus())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var addCalls int
	var removeCalls int
	ch.userFn = func() (*discordgo.User, error) {
		return &discordgo.User{ID: "bot-user", Username: "clawcord"}, nil
	}
	ch.openFn = func() error { return nil }
	ch.closeFn = func() error { return nil }
	ch.addHandlerFn = func(any) func() {
		addCalls++
		return func() { removeCalls++ }
	}

	if err := ch.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := ch.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := ch.Start(context.Background()); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if err := ch.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}

	if addCalls != 2 {
		t.Fatalf("addCalls = %d, want 2", addCalls)
	}
	if removeCalls != 2 {
		t.Fatalf("removeCalls = %d, want 2", removeCalls)
	}
}

func TestStartTypingBindsStopToTypingSession(t *testing.T) {
	ch, err := New(projectconfig.DiscordConfig{Token: "test-token"}, bus.NewMessageBus())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ch.typingFn = func(string) error { return nil }

	stop1, err := ch.StartTyping(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("StartTyping() error = %v", err)
	}
	stop2, err := ch.StartTyping(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("second StartTyping() error = %v", err)
	}

	session := ch.typingSessions["chat-1"]
	if session == nil {
		t.Fatal("typing session was not created")
	}
	if session.refs != 2 {
		t.Fatalf("session.refs = %d, want 2", session.refs)
	}

	stop1()

	session = ch.typingSessions["chat-1"]
	if session == nil {
		t.Fatal("typing session should still exist after first stop")
	}
	if session.refs != 1 {
		t.Fatalf("session.refs after first stop = %d, want 1", session.refs)
	}

	stop2()

	if _, ok := ch.typingSessions["chat-1"]; ok {
		t.Fatal("typing session should be removed after second stop")
	}
}

func TestStripBotMention(t *testing.T) {
	ch := &DiscordChannel{botUserID: "12345"}

	got := ch.stripBotMention("<@12345> hello <@!12345>")
	if got != "hello" {
		t.Fatalf("stripBotMention() = %q, want %q", got, "hello")
	}
}

func mustNewSession(t *testing.T) *discordgo.Session {
	t.Helper()

	session, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatalf("discordgo.New() error = %v", err)
	}
	if session.State == nil {
		t.Fatal("session.State is nil")
	}
	return session
}
