package gateway

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/argobell/clawcord/cmd/clawcord/internal"
	"github.com/argobell/clawcord/cmd/clawcord/internal/runtime"
	"github.com/argobell/clawcord/internal/agent"
	"github.com/argobell/clawcord/pkg/bus"
	"github.com/argobell/clawcord/pkg/channels"
	"github.com/argobell/clawcord/pkg/channels/discord"
	"github.com/argobell/clawcord/pkg/logger"
	"github.com/argobell/clawcord/pkg/media"
)

type gatewayChannel interface {
	outboundChannel
	Start(context.Context) error
	Stop(context.Context) error
	SetPlaceholderRecorder(channels.PlaceholderRecorder)
}

type gatewayRuntime struct {
	bus        *bus.MessageBus
	agent      *agent.AgentInstance
	channel    gatewayChannel
	controller *outboundController
	mediaStore *media.FileMediaStore
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func gatewayRun(ctx context.Context, flags gatewayFlags) error {
	if flags.Debug {
		logger.SetLevel(logger.DEBUG)
		fmt.Fprintln(os.Stderr, "Debug mode enabled")
	}

	rt, err := newGatewayRuntime(ctx)
	if err != nil {
		return err
	}
	defer rt.Close(context.Background())

	<-ctx.Done()
	return nil
}

func newGatewayRuntime(parent context.Context) (*gatewayRuntime, error) {
	cfg, err := internal.LoadConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Channels.Discord.Enabled {
		return nil, fmt.Errorf("discord channel is not enabled")
	}
	if cfg.Channels.Discord.Token == "" {
		return nil, fmt.Errorf("discord token is required")
	}

	agentCfg := runtime.ResolveDefaultAgent(cfg)
	instance, err := runtime.NewConfiguredAgentInstance(cfg, agentCfg, "")
	if err != nil {
		return nil, err
	}

	messageBus := bus.NewMessageBus()
	mediaStore := media.NewFileMediaStore()
	channel, err := discord.New(cfg.Channels.Discord, messageBus)
	if err != nil {
		_ = instance.Close()
		messageBus.Close()
		return nil, err
	}
	channel.SetMediaStore(mediaStore)
	// Gateway is the first runtime that can actually deliver media, so wire media-capable tools here.
	runtime.RegisterDefaultTools(instance.Tools, instance.Workspace, mediaStore)

	ctrl := newOutboundController(map[string]outboundChannel{
		channel.Name(): channel,
	})
	channel.SetPlaceholderRecorder(ctrl)

	ctx, cancel := context.WithCancel(parent)
	rt := &gatewayRuntime{
		bus:        messageBus,
		agent:      instance,
		channel:    channel,
		controller: ctrl,
		mediaStore: mediaStore,
		cancel:     cancel,
	}

	if err := channel.Start(ctx); err != nil {
		cancel()
		messageBus.Close()
		_ = instance.Close()
		return nil, err
	}

	rt.wg.Add(3)
	go func() {
		defer rt.wg.Done()
		runInboundLoop(ctx, rt.bus, rt.agent, rt.mediaStore)
	}()
	go func() {
		defer rt.wg.Done()
		runOutboundLoop(ctx, rt.bus, rt.controller)
	}()
	go func() {
		defer rt.wg.Done()
		runOutboundMediaLoop(ctx, rt.bus, rt.controller)
	}()

	return rt, nil
}

func (r *gatewayRuntime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}

	var firstErr error
	if r.cancel != nil {
		r.cancel()
	}
	if r.channel != nil {
		if err := r.channel.Stop(ctx); err != nil {
			firstErr = err
		}
	}
	if r.bus != nil {
		r.bus.Close()
	}
	r.wg.Wait()
	if r.agent != nil {
		if err := r.agent.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func runInboundLoop(ctx context.Context, b *bus.MessageBus, inst *agent.AgentInstance, store *media.FileMediaStore) {
	for {
		msg, ok := b.ConsumeInbound(ctx)
		if !ok {
			return
		}

		result, err := inst.RunTurn(ctx, agent.ProcessOptions{
			SessionKey:       msg.SessionKey,
			Channel:          msg.Channel,
			ChatID:           msg.ChatID,
			ReplyToMessageID: msg.MessageID,
			UserMessage:      msg.Content,
			Media:            msg.Media,
			MediaStore:       store,
			// Keep handled media delivery on the same bus path as text replies.
			PublishMedia: func(ctx context.Context, msg bus.OutboundMediaMessage) error {
				return b.PublishOutboundMedia(ctx, msg)
			},
		})
		if err != nil {
			logger.ErrorCF("gateway", "Agent turn failed", map[string]any{
				"channel": msg.Channel,
				"chat_id": msg.ChatID,
				"error":   err.Error(),
			})
			_ = b.PublishOutbound(ctx, bus.OutboundMessage{
				Channel:          msg.Channel,
				ChatID:           msg.ChatID,
				Content:          "Sorry, I hit an internal error.",
				ReplyToMessageID: msg.MessageID,
			})
			continue
		}
		if result == nil || result.Content == "" {
			continue
		}

		if err := b.PublishOutbound(ctx, bus.OutboundMessage{
			Channel:          msg.Channel,
			ChatID:           msg.ChatID,
			Content:          result.Content,
			ReplyToMessageID: msg.MessageID,
		}); err != nil {
			logger.ErrorCF("gateway", "Failed to publish outbound message", map[string]any{
				"channel": msg.Channel,
				"chat_id": msg.ChatID,
				"error":   err.Error(),
			})
		}
	}
}

func runOutboundLoop(ctx context.Context, b *bus.MessageBus, ctrl *outboundController) {
	for {
		msg, ok := b.SubscribeOutbound(ctx)
		if !ok {
			return
		}
		if err := ctrl.HandleOutbound(ctx, msg); err != nil {
			logger.ErrorCF("gateway", "Failed to deliver outbound message", map[string]any{
				"channel": msg.Channel,
				"chat_id": msg.ChatID,
				"error":   err.Error(),
			})
		}
	}
}

func runOutboundMediaLoop(ctx context.Context, b *bus.MessageBus, ctrl *outboundController) {
	for {
		msg, ok := b.SubscribeOutboundMedia(ctx)
		if !ok {
			return
		}
		if err := ctrl.HandleOutboundMedia(ctx, msg); err != nil {
			logger.ErrorCF("gateway", "Failed to deliver outbound media", map[string]any{
				"channel": msg.Channel,
				"chat_id": msg.ChatID,
				"error":   err.Error(),
			})
		}
	}
}
