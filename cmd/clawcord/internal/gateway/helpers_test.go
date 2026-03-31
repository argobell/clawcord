package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/argobell/clawcord/internal/agent"
	"github.com/argobell/clawcord/pkg/bus"
	"github.com/argobell/clawcord/pkg/channels"
	"github.com/argobell/clawcord/pkg/config"
	"github.com/argobell/clawcord/pkg/providers"
	"github.com/argobell/clawcord/pkg/session"
	"github.com/argobell/clawcord/pkg/tools"
)

type fakeGatewayProvider struct {
	content string
}

func (f *fakeGatewayProvider) Chat(
	_ context.Context,
	_ []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: f.content}, nil
}

func (f *fakeGatewayProvider) GetDefaultModel() string { return "gpt-5.4-mini" }

func TestRunInboundLoopPublishesOutboundReply(t *testing.T) {
	messageBus := bus.NewMessageBus()
	defer messageBus.Close()

	instance, err := agent.NewAgentInstance(
		config.AgentConfig{ID: "main"},
		config.AgentDefaults{ModelName: "main"},
		&config.Config{
			ModelList: []config.ModelConfig{{ModelName: "main", Model: "gpt-5.4-mini"}},
		},
		&fakeGatewayProvider{content: "pong"},
		session.NewSessionManager(t.TempDir()),
		tools.NewToolRegistry(),
	)
	if err != nil {
		t.Fatalf("NewAgentInstance() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		runInboundLoop(ctx, messageBus, instance, nil)
		close(done)
	}()

	err = messageBus.PublishInbound(ctx, bus.InboundMessage{
		Channel:    "discord",
		ChatID:     "chat-1",
		Content:    "ping",
		SessionKey: "discord:direct:user-1",
		MessageID:  "msg-1",
	})
	if err != nil {
		t.Fatalf("PublishInbound() error = %v", err)
	}

	got, ok := messageBus.SubscribeOutbound(ctx)
	if !ok {
		t.Fatal("expected outbound message")
	}
	if got.Channel != "discord" {
		t.Fatalf("Channel = %q, want discord", got.Channel)
	}
	if got.ChatID != "chat-1" {
		t.Fatalf("ChatID = %q, want chat-1", got.ChatID)
	}
	if got.Content != "pong" {
		t.Fatalf("Content = %q, want pong", got.Content)
	}
	if got.ReplyToMessageID != "msg-1" {
		t.Fatalf("ReplyToMessageID = %q, want msg-1", got.ReplyToMessageID)
	}

	cancel()
	<-done
}

type gatewaySessionStore struct {
	closeCalls int
}

func (f *gatewaySessionStore) AddMessage(_, _, _ string)                    {}
func (f *gatewaySessionStore) AddFullMessage(_ string, _ providers.Message) {}
func (f *gatewaySessionStore) GetHistory(_ string) []providers.Message      { return nil }
func (f *gatewaySessionStore) GetSummary(_ string) string                   { return "" }
func (f *gatewaySessionStore) SetSummary(_, _ string)                       {}
func (f *gatewaySessionStore) SetHistory(_ string, _ []providers.Message)   {}
func (f *gatewaySessionStore) TruncateHistory(_ string, _ int)              {}
func (f *gatewaySessionStore) Save(_ string) error                          { return nil }
func (f *gatewaySessionStore) Close() error {
	f.closeCalls++
	return nil
}

type fakeGatewayChannel struct {
	name                   string
	stopErr                error
	stopCalls              int
	placeholderRecorderSet bool
}

func (f *fakeGatewayChannel) Name() string                { return f.name }
func (f *fakeGatewayChannel) Start(context.Context) error { return nil }
func (f *fakeGatewayChannel) Stop(context.Context) error {
	f.stopCalls++
	return f.stopErr
}
func (f *fakeGatewayChannel) Send(context.Context, bus.OutboundMessage) error { return nil }
func (f *fakeGatewayChannel) IsRunning() bool                                 { return true }
func (f *fakeGatewayChannel) IsAllowed(string) bool                           { return true }
func (f *fakeGatewayChannel) IsAllowedSender(bus.SenderInfo) bool             { return true }
func (f *fakeGatewayChannel) ReasoningChannelID() string                      { return "" }
func (f *fakeGatewayChannel) SetPlaceholderRecorder(_ channels.PlaceholderRecorder) {
	f.placeholderRecorderSet = true
}

func TestGatewayRuntimeCloseContinuesCleanupOnChannelStopError(t *testing.T) {
	stopErr := errors.New("close failed")
	messageBus := bus.NewMessageBus()
	ch := &fakeGatewayChannel{name: "discord", stopErr: stopErr}

	store := &gatewaySessionStore{}
	inst, err := agent.NewAgentInstance(
		config.AgentConfig{ID: "main"},
		config.AgentDefaults{ModelName: "main"},
		&config.Config{
			ModelList: []config.ModelConfig{{ModelName: "main", Model: "gpt-5.4-mini"}},
		},
		&fakeGatewayProvider{content: "pong"},
		store,
		tools.NewToolRegistry(),
	)
	if err != nil {
		t.Fatalf("NewAgentInstance() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rt := &gatewayRuntime{
		bus:     messageBus,
		agent:   inst,
		channel: ch,
		cancel:  cancel,
	}
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		<-ctx.Done()
	}()

	err = rt.Close(context.Background())
	if !errors.Is(err, stopErr) {
		t.Fatalf("Close() error = %v, want %v", err, stopErr)
	}
	if store.closeCalls != 1 {
		t.Fatalf("session Close calls = %d, want 1", store.closeCalls)
	}
	if ch.stopCalls != 1 {
		t.Fatalf("channel stop calls = %d, want 1", ch.stopCalls)
	}
	if publishErr := messageBus.PublishOutbound(context.Background(), bus.OutboundMessage{}); !errors.Is(publishErr, bus.ErrBusClosed) {
		t.Fatalf("PublishOutbound() error = %v, want ErrBusClosed", publishErr)
	}
}
