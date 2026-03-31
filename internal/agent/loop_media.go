package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/argobell/clawcord/pkg/bus"
	"github.com/argobell/clawcord/pkg/logger"
	"github.com/argobell/clawcord/pkg/media"
	"github.com/argobell/clawcord/pkg/providers"
)

const defaultMaxMediaSize = 20 * 1024 * 1024

// resolveMediaRefs resolves media:// refs before provider calls.
// 当前先只把图片转换为 data URL，保持最小可用的多模态输入链路。
func resolveMediaRefs(messages []providers.Message, store media.MediaStore, maxSize int) []providers.Message {
	if store == nil {
		return messages
	}

	result := make([]providers.Message, len(messages))
	copy(result, messages)

	for i, msg := range result {
		if len(msg.Media) == 0 {
			continue
		}

		resolved := make([]string, 0, len(msg.Media))
		for _, ref := range msg.Media {
			if !strings.HasPrefix(ref, "media://") {
				resolved = append(resolved, ref)
				continue
			}

			localPath, meta, err := store.ResolveWithMeta(ref)
			if err != nil {
				logger.WarnCF("agent", "Failed to resolve media ref", map[string]any{
					"ref":   ref,
					"error": err.Error(),
				})
				continue
			}

			if !strings.HasPrefix(strings.ToLower(meta.ContentType), "image/") {
				continue
			}

			dataURL := encodeImageToDataURL(localPath, meta.ContentType, maxSize)
			if dataURL != "" {
				resolved = append(resolved, dataURL)
			}
		}
		result[i].Media = resolved
	}

	return result
}

func encodeImageToDataURL(localPath, contentType string, maxSize int) string {
	info, err := os.Stat(localPath)
	if err != nil {
		logger.WarnCF("agent", "Failed to stat media file", map[string]any{
			"path":  localPath,
			"error": err.Error(),
		})
		return ""
	}
	if info.Size() > int64(maxSize) {
		logger.WarnCF("agent", "Media file too large, skipping", map[string]any{
			"path":     localPath,
			"size":     info.Size(),
			"max_size": maxSize,
		})
		return ""
	}

	file, err := os.Open(localPath)
	if err != nil {
		logger.WarnCF("agent", "Failed to open media file", map[string]any{
			"path":  localPath,
			"error": err.Error(),
		})
		return ""
	}
	defer file.Close()

	prefix := "data:" + contentType + ";base64,"
	encodedLen := base64.StdEncoding.EncodedLen(int(info.Size()))
	var buf bytes.Buffer
	buf.Grow(len(prefix) + encodedLen)
	buf.WriteString(prefix)

	encoder := base64.NewEncoder(base64.StdEncoding, &buf)
	if _, err := io.Copy(encoder, file); err != nil {
		logger.WarnCF("agent", "Failed to encode media file", map[string]any{
			"path":  localPath,
			"error": err.Error(),
		})
		return ""
	}
	if err := encoder.Close(); err != nil {
		logger.WarnCF("agent", "Failed to finalize media encoding", map[string]any{
			"path":  localPath,
			"error": err.Error(),
		})
		return ""
	}

	return buf.String()
}

func inferMediaType(filename, contentType string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/"):
		return "image"
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "audio/"):
		return "audio"
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "video/"):
		return "video"
	}

	switch strings.ToLower(filepath.Ext(strings.TrimSpace(filename))) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return "image"
	case ".mp3", ".wav", ".m4a", ".ogg":
		return "audio"
	case ".mp4", ".mov", ".webm":
		return "video"
	default:
		return "file"
	}
}

func publishToolMedia(ctx context.Context, opts ProcessOptions, refs []string) error {
	if len(refs) == 0 || opts.PublishMedia == nil {
		return nil
	}

	// Preserve filename and MIME hints so channel adapters can choose the right upload behavior.
	outbound := bus.OutboundMediaMessage{
		Channel:          opts.Channel,
		ChatID:           opts.ChatID,
		ReplyToMessageID: opts.ReplyToMessageID,
		Parts:            make([]bus.MediaPart, 0, len(refs)),
	}
	for _, ref := range refs {
		part := bus.MediaPart{Ref: ref}
		if opts.MediaStore != nil {
			if _, meta, err := opts.MediaStore.ResolveWithMeta(ref); err == nil {
				part.Filename = meta.Filename
				part.ContentType = meta.ContentType
				part.Type = inferMediaType(meta.Filename, meta.ContentType)
			}
		}
		outbound.Parts = append(outbound.Parts, part)
	}

	if err := opts.PublishMedia(ctx, outbound); err != nil {
		return fmt.Errorf("publish outbound media failed: %w", err)
	}
	return nil
}
