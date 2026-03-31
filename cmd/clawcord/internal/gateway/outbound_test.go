package gateway

import (
	"context"
	"testing"

	"github.com/argobell/clawcord/pkg/bus"
)

type fakeOutboundChannel struct {
	name        string
	sendCalls   []bus.OutboundMessage
	mediaCalls  []bus.OutboundMediaMessage
	editCalls   []editCall
	running     bool
	reasoningID string
}

type editCall struct {
	chatID    string
	messageID string
	content   string
}

func (f *fakeOutboundChannel) Name() string                { return f.name }
func (f *fakeOutboundChannel) Start(context.Context) error { f.running = true; return nil }
func (f *fakeOutboundChannel) Stop(context.Context) error  { f.running = false; return nil }
func (f *fakeOutboundChannel) Send(_ context.Context, msg bus.OutboundMessage) error {
	f.sendCalls = append(f.sendCalls, msg)
	return nil
}
func (f *fakeOutboundChannel) SendMedia(_ context.Context, msg bus.OutboundMediaMessage) error {
	f.mediaCalls = append(f.mediaCalls, msg)
	return nil
}
func (f *fakeOutboundChannel) IsRunning() bool                     { return f.running }
func (f *fakeOutboundChannel) IsAllowed(string) bool               { return true }
func (f *fakeOutboundChannel) IsAllowedSender(bus.SenderInfo) bool { return true }
func (f *fakeOutboundChannel) ReasoningChannelID() string          { return f.reasoningID }
func (f *fakeOutboundChannel) EditMessage(_ context.Context, chatID string, messageID string, content string) error {
	f.editCalls = append(f.editCalls, editCall{chatID: chatID, messageID: messageID, content: content})
	return nil
}

func TestOutboundControllerEditsPlaceholderAndStopsTyping(t *testing.T) {
	channel := &fakeOutboundChannel{name: "discord", running: true}
	ctrl := newOutboundController(map[string]outboundChannel{
		"discord": channel,
	})

	stopCalls := 0
	ctrl.RecordPlaceholder("discord", "chat-1", "msg-1", "placeholder-1")
	ctrl.RecordTypingStop("discord", "chat-1", "msg-1", func() { stopCalls++ })

	err := ctrl.HandleOutbound(context.Background(), bus.OutboundMessage{
		Channel:          "discord",
		ChatID:           "chat-1",
		Content:          "hello",
		ReplyToMessageID: "msg-1",
	})
	if err != nil {
		t.Fatalf("HandleOutbound() error = %v", err)
	}

	if stopCalls != 1 {
		t.Fatalf("stopCalls = %d, want 1", stopCalls)
	}
	if len(channel.editCalls) != 1 {
		t.Fatalf("editCalls = %d, want 1", len(channel.editCalls))
	}
	if len(channel.sendCalls) != 0 {
		t.Fatalf("sendCalls = %d, want 0", len(channel.sendCalls))
	}
}

func TestOutboundControllerFallsBackToSend(t *testing.T) {
	channel := &fakeOutboundChannel{name: "discord", running: true}
	ctrl := newOutboundController(map[string]outboundChannel{
		"discord": channel,
	})

	err := ctrl.HandleOutbound(context.Background(), bus.OutboundMessage{
		Channel: "discord",
		ChatID:  "chat-1",
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("HandleOutbound() error = %v", err)
	}

	if len(channel.sendCalls) != 1 {
		t.Fatalf("sendCalls = %d, want 1", len(channel.sendCalls))
	}
	if len(channel.editCalls) != 0 {
		t.Fatalf("editCalls = %d, want 0", len(channel.editCalls))
	}
}

func TestOutboundControllerMatchesPlaceholderPerMessage(t *testing.T) {
	channel := &fakeOutboundChannel{name: "discord", running: true}
	ctrl := newOutboundController(map[string]outboundChannel{
		"discord": channel,
	})

	stopCalls := 0
	ctrl.RecordPlaceholder("discord", "chat-1", "msg-1", "placeholder-1")
	ctrl.RecordTypingStop("discord", "chat-1", "msg-1", func() { stopCalls++ })
	ctrl.RecordPlaceholder("discord", "chat-1", "msg-2", "placeholder-2")
	ctrl.RecordTypingStop("discord", "chat-1", "msg-2", func() { stopCalls++ })

	err := ctrl.HandleOutbound(context.Background(), bus.OutboundMessage{
		Channel:          "discord",
		ChatID:           "chat-1",
		Content:          "second",
		ReplyToMessageID: "msg-2",
	})
	if err != nil {
		t.Fatalf("HandleOutbound() error = %v", err)
	}

	if stopCalls != 1 {
		t.Fatalf("stopCalls = %d, want 1", stopCalls)
	}
	if len(channel.editCalls) != 1 {
		t.Fatalf("editCalls = %d, want 1", len(channel.editCalls))
	}
	if channel.editCalls[0].messageID != "placeholder-2" {
		t.Fatalf("edited placeholder = %q, want placeholder-2", channel.editCalls[0].messageID)
	}
}

func TestOutboundControllerSendsMedia(t *testing.T) {
	channel := &fakeOutboundChannel{name: "discord", running: true}
	ctrl := newOutboundController(map[string]outboundChannel{
		"discord": channel,
	})

	stopCalls := 0
	ctrl.RecordPlaceholder("discord", "chat-1", "msg-1", "placeholder-1")
	ctrl.RecordTypingStop("discord", "chat-1", "msg-1", func() { stopCalls++ })

	err := ctrl.HandleOutboundMedia(context.Background(), bus.OutboundMediaMessage{
		Channel:          "discord",
		ChatID:           "chat-1",
		ReplyToMessageID: "msg-1",
		Parts: []bus.MediaPart{
			{Ref: "media://abc"},
		},
	})
	if err != nil {
		t.Fatalf("HandleOutboundMedia() error = %v", err)
	}
	if len(channel.mediaCalls) != 1 {
		t.Fatalf("mediaCalls = %d, want 1", len(channel.mediaCalls))
	}
	if stopCalls != 1 {
		t.Fatalf("stopCalls = %d, want 1", stopCalls)
	}
	if channel.mediaCalls[0].Parts[0].Ref != "media://abc" {
		t.Fatalf("media ref = %q, want media://abc", channel.mediaCalls[0].Parts[0].Ref)
	}
}
