package channels

import (
	"context"

	"github.com/argobell/clawcord/pkg/bus"
)

// MediaSender is an optional interface for channels that can send media attachments.
type MediaSender interface {
	SendMedia(ctx context.Context, msg bus.OutboundMediaMessage) error
}
