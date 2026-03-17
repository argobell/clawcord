package bus

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/argobell/clawcord/pkg/logger"
)

// ErrBusClosed 在向已关闭的 MessageBus 发布时返回
var ErrBusClosed = errors.New("message bus closed")

// defaultBusBufferSize 是 MessageBus 内部通道的默认缓冲大小
// 允许一定程度的异步处理而不会过快阻塞
const defaultBusBufferSize = 64

// MessageBus 提供了一个简单的发布-订阅机制，用于在 Agent 和频道之间传递消息
// 支持同时处理文本和媒体消息，并且设计为线程安全的，可以安全地在多个 goroutine 之间使用。
type MessageBus struct {
	inbound       chan InboundMessage
	outbound      chan OutboundMessage
	outboundMedia chan OutboundMediaMessage
	done          chan struct{}
	closed        atomic.Bool
}

// NewMessageBus 创建并初始化一个新的 MessageBus 实例，准备好处理消息传递。
func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:       make(chan InboundMessage, defaultBusBufferSize),
		outbound:      make(chan OutboundMessage, defaultBusBufferSize),
		outboundMedia: make(chan OutboundMediaMessage, defaultBusBufferSize),
		done:          make(chan struct{}),
	}
}

// PublishInbound 将一个 InboundMessage 发布到 MessageBus，供 Agent 消费。
// 如果 MessageBus 已关闭或上下文已取消，将返回相应的错误。
func (mb *MessageBus) PublishInbound(ctx context.Context, msg InboundMessage) error {
	if mb.closed.Load() {
		return ErrBusClosed
	}
	// 在尝试发布消息之前检查上下文是否已取消，以避免在不必要的情况下阻塞。
	if err := ctx.Err(); err != nil {
		return err
	}
	// 使用 select 语句同时监听多个通道，确保在 MessageBus 关闭或上下文取消时能够及时响应
	select {
	case mb.inbound <- msg:
		return nil
	case <-mb.done:
		return ErrBusClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ConsumeInbound 从 MessageBus 订阅并返回一个 InboundMessage，供 Agent 消费。
// 如果 MessageBus 已关闭或上下文已取消，将返回一个零值消息和 false。
func (mb *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, bool) {
	select {
	case msg, ok := <-mb.inbound:
		return msg, ok
	case <-mb.done:
		return InboundMessage{}, false
	case <-ctx.Done():
		return InboundMessage{}, false
	}
}

// PublishOutbound 将一个 OutboundMessage 发布到 MessageBus，供频道消费。
// 如果 MessageBus 已关闭或上下文已取消，将返回相应的错误。
func (mb *MessageBus) PublishOutbound(ctx context.Context, msg OutboundMessage) error {
	if mb.closed.Load() {
		return ErrBusClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case mb.outbound <- msg:
		return nil
	case <-mb.done:
		return ErrBusClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SubscribeOutbound 从 MessageBus 订阅并返回一个 OutboundMessage，供频道消费。
// 如果 MessageBus 已关闭或上下文已取消，将返回一个零值消息和 false。
func (mb *MessageBus) SubscribeOutbound(ctx context.Context) (OutboundMessage, bool) {
	select {
	case msg, ok := <-mb.outbound:
		return msg, ok
	case <-mb.done:
		return OutboundMessage{}, false
	case <-ctx.Done():
		return OutboundMessage{}, false
	}
}

// PublishOutboundMedia 将一个 OutboundMediaMessage 发布到 MessageBus，供频道消费。
// 如果 MessageBus 已关闭或上下文已取消，将返回相应的错误。
func (mb *MessageBus) PublishOutboundMedia(ctx context.Context, msg OutboundMediaMessage) error {
	if mb.closed.Load() {
		return ErrBusClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case mb.outboundMedia <- msg:
		return nil
	case <-mb.done:
		return ErrBusClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SubscribeOutboundMedia 从 MessageBus 订阅并返回一个 OutboundMediaMessage，供频道消费。
// 如果 MessageBus 已关闭或上下文已取消，将返回一个零值消息和 false。
func (mb *MessageBus) SubscribeOutboundMedia(ctx context.Context) (OutboundMediaMessage, bool) {
	select {
	case msg, ok := <-mb.outboundMedia:
		return msg, ok
	case <-mb.done:
		return OutboundMediaMessage{}, false
	case <-ctx.Done():
		return OutboundMediaMessage{}, false
	}
}

// Close 安全地关闭 MessageBus，确保所有资源得到正确释放，并且在关闭后不再接受新的消息发布。
func (mb *MessageBus) Close() {
	if mb.closed.CompareAndSwap(false, true) {
		close(mb.done)

		// Drain buffered channels so messages aren't silently lost.
		// Channels are NOT closed to avoid send-on-closed panics from concurrent publishers.
		drained := 0
		for {
			select {
			case <-mb.inbound:
				drained++
			default:
				goto doneInbound
			}
		}
	doneInbound:
		for {
			select {
			case <-mb.outbound:
				drained++
			default:
				goto doneOutbound
			}
		}
	doneOutbound:
		for {
			select {
			case <-mb.outboundMedia:
				drained++
			default:
				goto doneMedia
			}
		}
	doneMedia:
		if drained > 0 {
			logger.DebugCF("bus", "Drained buffered messages during close", map[string]any{
				"count": drained,
			})
		}
	}
}
