package channels

import (
	"context"
	"testing"

	"github.com/argobell/clawcord/pkg/bus"
	projectconfig "github.com/argobell/clawcord/pkg/config"
)

func TestBaseChannelIsAllowed(t *testing.T) {
	tests := []struct {
		name      string
		allowList []string
		senderID  string
		want      bool
	}{
		{
			name:      "empty allowlist allows all",
			allowList: nil,
			senderID:  "anyone",
			want:      true,
		},
		{
			name:      "compound sender matches numeric allowlist",
			allowList: []string{"123456"},
			senderID:  "123456|alice",
			want:      true,
		},
		{
			name:      "compound sender matches username allowlist",
			allowList: []string{"@alice"},
			senderID:  "123456|alice",
			want:      true,
		},
		{
			name:      "numeric sender matches legacy compound allowlist",
			allowList: []string{"123456|alice"},
			senderID:  "123456",
			want:      true,
		},
		{
			name:      "non matching sender is denied",
			allowList: []string{"123456"},
			senderID:  "654321|bob",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewBaseChannel("discord", nil, nil, tt.allowList)
			if got := ch.IsAllowed(tt.senderID); got != tt.want {
				t.Fatalf("IsAllowed(%q) = %v, want %v", tt.senderID, got, tt.want)
			}
		})
	}
}

func TestShouldRespondInGroup(t *testing.T) {
	tests := []struct {
		name        string
		gt          projectconfig.GroupTriggerConfig
		isMentioned bool
		content     string
		wantRespond bool
		wantContent string
	}{
		{
			name:        "no config is permissive",
			gt:          projectconfig.GroupTriggerConfig{},
			isMentioned: false,
			content:     "hello world",
			wantRespond: true,
			wantContent: "hello world",
		},
		{
			name:        "mention only blocks unmentioned messages",
			gt:          projectconfig.GroupTriggerConfig{MentionOnly: true},
			isMentioned: false,
			content:     "hello world",
			wantRespond: false,
			wantContent: "hello world",
		},
		{
			name:        "mention always responds",
			gt:          projectconfig.GroupTriggerConfig{MentionOnly: true},
			isMentioned: true,
			content:     "hello world",
			wantRespond: true,
			wantContent: "hello world",
		},
		{
			name:        "prefix match strips prefix",
			gt:          projectconfig.GroupTriggerConfig{Prefixes: []string{"/ask"}},
			isMentioned: false,
			content:     "/ask hello",
			wantRespond: true,
			wantContent: "hello",
		},
		{
			name:        "prefix miss blocks message",
			gt:          projectconfig.GroupTriggerConfig{Prefixes: []string{"/ask"}},
			isMentioned: false,
			content:     "hello",
			wantRespond: false,
			wantContent: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewBaseChannel("discord", nil, nil, nil, WithGroupTrigger(tt.gt))
			gotRespond, gotContent := ch.ShouldRespondInGroup(tt.isMentioned, tt.content)
			if gotRespond != tt.wantRespond {
				t.Fatalf("ShouldRespondInGroup() respond = %v, want %v", gotRespond, tt.wantRespond)
			}
			if gotContent != tt.wantContent {
				t.Fatalf("ShouldRespondInGroup() content = %q, want %q", gotContent, tt.wantContent)
			}
		})
	}
}

func TestBaseChannelIsAllowedSender(t *testing.T) {
	tests := []struct {
		name      string
		allowList []string
		sender    bus.SenderInfo
		want      bool
	}{
		{
			name:      "empty allowlist allows all",
			allowList: nil,
			sender:    bus.SenderInfo{PlatformID: "anyone"},
			want:      true,
		},
		{
			name:      "platform id matches raw id",
			allowList: []string{"123456"},
			sender: bus.SenderInfo{
				Platform:    "discord",
				PlatformID:  "123456",
				CanonicalID: "discord:123456",
			},
			want: true,
		},
		{
			name:      "canonical format matches",
			allowList: []string{"discord:123456"},
			sender: bus.SenderInfo{
				Platform:    "discord",
				PlatformID:  "123456",
				CanonicalID: "discord:123456",
			},
			want: true,
		},
		{
			name:      "username matches",
			allowList: []string{"@alice"},
			sender: bus.SenderInfo{
				Platform:    "discord",
				PlatformID:  "123456",
				CanonicalID: "discord:123456",
				Username:    "alice",
			},
			want: true,
		},
		{
			name:      "non matching sender is denied",
			allowList: []string{"discord:999"},
			sender: bus.SenderInfo{
				Platform:    "discord",
				PlatformID:  "123456",
				CanonicalID: "discord:123456",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewBaseChannel("discord", nil, nil, tt.allowList)
			if got := ch.IsAllowedSender(tt.sender); got != tt.want {
				t.Fatalf("IsAllowedSender(%+v) = %v, want %v", tt.sender, got, tt.want)
			}
		})
	}
}

func TestHandleMessagePublishesInboundMessage(t *testing.T) {
	messageBus := bus.NewMessageBus()
	ch := NewBaseChannel("discord", nil, messageBus, nil)

	sender := bus.SenderInfo{
		Platform:    "discord",
		PlatformID:  "123456",
		CanonicalID: "discord:123456",
		Username:    "alice",
		DisplayName: "Alice",
	}

	ch.HandleMessage(
		context.Background(),
		bus.Peer{Kind: "direct", ID: "dm:123456"},
		"msg-1",
		"123456",
		"chat-1",
		"hello",
		[]string{"media://1"},
		map[string]string{"thread_id": "abc"},
		sender,
	)

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
	if msg.ChatID != "chat-1" {
		t.Fatalf("ChatID = %q, want %q", msg.ChatID, "chat-1")
	}
	if msg.Content != "hello" {
		t.Fatalf("Content = %q, want %q", msg.Content, "hello")
	}
	if msg.MessageID != "msg-1" {
		t.Fatalf("MessageID = %q, want %q", msg.MessageID, "msg-1")
	}
	if msg.MediaScope != "discord:chat-1:msg-1" {
		t.Fatalf("MediaScope = %q, want %q", msg.MediaScope, "discord:chat-1:msg-1")
	}
	if msg.SessionKey != "discord:direct:dm:123456" {
		t.Fatalf("SessionKey = %q, want %q", msg.SessionKey, "discord:direct:dm:123456")
	}
	if got := msg.Metadata["thread_id"]; got != "abc" {
		t.Fatalf("Metadata[thread_id] = %q, want %q", got, "abc")
	}
}

func TestHandleMessageRecordsTypingAndPlaceholder(t *testing.T) {
	messageBus := bus.NewMessageBus()
	recorder := &recordingPlaceholderRecorder{}

	ch := NewBaseChannel("discord", nil, messageBus, nil)
	owner := &testOwnerChannel{}
	ch.SetOwner(owner)
	ch.SetPlaceholderRecorder(recorder)

	ch.HandleMessage(
		context.Background(),
		bus.Peer{Kind: "direct", ID: "dm:123456"},
		"msg-1",
		"123456",
		"chat-1",
		"hello",
		nil,
		nil,
		bus.SenderInfo{
			Platform:    "discord",
			PlatformID:  "123456",
			CanonicalID: "discord:123456",
		},
	)

	if recorder.typingChannel != "discord" || recorder.typingChatID != "chat-1" || recorder.typingStop == nil {
		t.Fatalf("typing recorder not populated: %+v", recorder)
	}
	if recorder.typingMessageID != "msg-1" {
		t.Fatalf("typing message id = %q, want %q", recorder.typingMessageID, "msg-1")
	}
	if recorder.placeholderChannel != "discord" || recorder.placeholderChatID != "chat-1" || recorder.placeholderID != "placeholder-1" {
		t.Fatalf("placeholder recorder not populated: %+v", recorder)
	}
	if recorder.placeholderMessageID != "msg-1" {
		t.Fatalf("placeholder message id = %q, want %q", recorder.placeholderMessageID, "msg-1")
	}
}

type recordingPlaceholderRecorder struct {
	placeholderChannel   string
	placeholderChatID    string
	placeholderMessageID string
	placeholderID        string
	typingChannel        string
	typingChatID         string
	typingMessageID      string
	typingStop           func()
}

func (r *recordingPlaceholderRecorder) RecordPlaceholder(channel, chatID, messageID, placeholderID string) {
	r.placeholderChannel = channel
	r.placeholderChatID = chatID
	r.placeholderMessageID = messageID
	r.placeholderID = placeholderID
}

func (r *recordingPlaceholderRecorder) RecordTypingStop(channel, chatID, messageID string, stop func()) {
	r.typingChannel = channel
	r.typingChatID = chatID
	r.typingMessageID = messageID
	r.typingStop = stop
}

type testOwnerChannel struct{}

func (c *testOwnerChannel) Name() string { return "discord" }

func (c *testOwnerChannel) Start(context.Context) error { return nil }

func (c *testOwnerChannel) Stop(context.Context) error { return nil }

func (c *testOwnerChannel) Send(context.Context, bus.OutboundMessage) error { return nil }

func (c *testOwnerChannel) IsRunning() bool { return true }

func (c *testOwnerChannel) IsAllowed(string) bool { return true }

func (c *testOwnerChannel) IsAllowedSender(bus.SenderInfo) bool { return true }

func (c *testOwnerChannel) ReasoningChannelID() string { return "" }

func (c *testOwnerChannel) StartTyping(context.Context, string) (func(), error) {
	return func() {}, nil
}

func (c *testOwnerChannel) SendPlaceholder(context.Context, string) (string, error) {
	return "placeholder-1", nil
}
