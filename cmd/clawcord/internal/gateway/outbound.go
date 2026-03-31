package gateway

import (
	"context"
	"fmt"
	"sync"

	"github.com/argobell/clawcord/pkg/bus"
	"github.com/argobell/clawcord/pkg/channels"
)

type outboundChannel interface {
	channels.Channel
}

type outboundController struct {
	channels     map[string]outboundChannel
	mu           sync.Mutex
	placeholders map[string]string
	typingStops  map[string]func()
}

func newOutboundController(channelsByName map[string]outboundChannel) *outboundController {
	return &outboundController{
		channels:     channelsByName,
		placeholders: make(map[string]string),
		typingStops:  make(map[string]func()),
	}
}

func (c *outboundController) RecordPlaceholder(channel, chatID, messageID, placeholderID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.placeholders[outboundKey(channel, chatID, messageID)] = placeholderID
}

func (c *outboundController) RecordTypingStop(channel, chatID, messageID string, stop func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.typingStops[outboundKey(channel, chatID, messageID)] = stop
}

func (c *outboundController) HandleOutbound(ctx context.Context, msg bus.OutboundMessage) error {
	ch, ok := c.channels[msg.Channel]
	if !ok {
		return fmt.Errorf("channel %q not found", msg.Channel)
	}

	key := outboundKey(msg.Channel, msg.ChatID, msg.ReplyToMessageID)

	c.mu.Lock()
	stop := c.typingStops[key]
	delete(c.typingStops, key)
	placeholderID := c.placeholders[key]
	if placeholderID != "" {
		delete(c.placeholders, key)
	}
	c.mu.Unlock()

	if stop != nil {
		stop()
	}

	if placeholderID != "" {
		if editor, ok := ch.(channels.MessageEditor); ok {
			if err := editor.EditMessage(ctx, msg.ChatID, placeholderID, msg.Content); err == nil {
				return nil
			}
		}
	}

	return ch.Send(ctx, msg)
}

func (c *outboundController) HandleOutboundMedia(ctx context.Context, msg bus.OutboundMediaMessage) error {
	ch, ok := c.channels[msg.Channel]
	if !ok {
		return fmt.Errorf("channel %q not found", msg.Channel)
	}

	key := outboundKey(msg.Channel, msg.ChatID, msg.ReplyToMessageID)

	c.mu.Lock()
	stop := c.typingStops[key]
	delete(c.typingStops, key)
	delete(c.placeholders, key)
	c.mu.Unlock()

	if stop != nil {
		stop()
	}

	if sender, ok := ch.(channels.MediaSender); ok {
		return sender.SendMedia(ctx, msg)
	}
	return fmt.Errorf("channel %q cannot send media", msg.Channel)
}

func outboundKey(channel, chatID, messageID string) string {
	return channel + ":" + chatID + ":" + messageID
}
