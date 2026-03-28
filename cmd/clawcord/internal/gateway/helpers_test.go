package gateway

import (
	"context"
	"testing"

	internalagent "github.com/argobell/clawcord/internal/agent"
	projectconfig "github.com/argobell/clawcord/pkg/config"
	"github.com/argobell/clawcord/pkg/bus"
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

	instance, err := internalagent.NewAgentInstance(
		projectconfig.AgentConfig{ID: "main"},
		projectconfig.AgentDefaults{ModelName: "main"},
		&projectconfig.Config{
			ModelList: []projectconfig.ModelConfig{{ModelName: "main", Model: "gpt-5.4-mini"}},
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
		runInboundLoop(ctx, messageBus, instance)
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
