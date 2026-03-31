package builtintools

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/h2non/filetype"

	"github.com/argobell/clawcord/pkg/media"
	pkgtools "github.com/argobell/clawcord/pkg/tools"
)

const defaultSendFileMaxSize = 20 * 1024 * 1024

// SendFileTool registers a local file in the media store for delivery to the current chat.
type SendFileTool struct {
	workspace   string
	maxFileSize int
	mediaStore  media.MediaStore
}

func NewSendFileTool(workspace string, maxFileSize int, store media.MediaStore) *SendFileTool {
	if maxFileSize <= 0 {
		maxFileSize = defaultSendFileMaxSize
	}
	return &SendFileTool{
		workspace:   strings.TrimSpace(workspace),
		maxFileSize: maxFileSize,
		mediaStore:  store,
	}
}

func (t *SendFileTool) Name() string { return "send_file" }

func (t *SendFileTool) Description() string {
	return "Send a local file to the user on the current chat channel."
}

func (t *SendFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the local file. Relative paths are resolved from workspace.",
			},
			"filename": map[string]any{
				"type":        "string",
				"description": "Optional display filename. Defaults to the basename of path.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *SendFileTool) Execute(ctx context.Context, args map[string]any) *pkgtools.ToolResult {
	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" {
		return pkgtools.ErrorResult("path is required")
	}

	channel := pkgtools.ToolChannel(ctx)
	chatID := pkgtools.ToolChatID(ctx)
	if channel == "" || chatID == "" {
		return pkgtools.ErrorResult("no target channel/chat available")
	}
	if t.mediaStore == nil {
		return pkgtools.ErrorResult("media store not configured")
	}

	resolved := t.resolvePath(path)

	info, err := os.Stat(resolved)
	if err != nil {
		return pkgtools.ErrorResult(fmt.Sprintf("file not found: %v", err))
	}
	if info.IsDir() {
		return pkgtools.ErrorResult("path is a directory, expected a file")
	}
	if info.Size() > int64(t.maxFileSize) {
		return pkgtools.ErrorResult(fmt.Sprintf("file too large: %d bytes (max %d bytes)", info.Size(), t.maxFileSize))
	}

	filename, _ := args["filename"].(string)
	if strings.TrimSpace(filename) == "" {
		filename = filepath.Base(resolved)
	}

	// The scope ties all refs produced for this chat to one logical cleanup bucket.
	ref, err := t.mediaStore.Store(resolved, media.MediaMeta{
		Filename:    filename,
		ContentType: detectMediaType(resolved),
		Source:      "tool:send_file",
	}, fmt.Sprintf("tool:send_file:%s:%s", channel, chatID))
	if err != nil {
		return pkgtools.ErrorResult(fmt.Sprintf("failed to register media: %v", err))
	}

	return pkgtools.MediaResult(fmt.Sprintf("File %q sent to user", filename), []string{ref}).WithResponseHandled()
}

func (t *SendFileTool) resolvePath(path string) string {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	base := t.workspace
	if base == "" {
		base = "."
	}
	return filepath.Join(base, path)
}

func detectMediaType(path string) string {
	// Match file content first so the tool still works when extensions are missing or misleading.
	kind, err := filetype.MatchFile(path)
	if err == nil && kind != filetype.Unknown {
		return kind.MIME.Value
	}
	if ext := filepath.Ext(path); ext != "" {
		if contentType := mime.TypeByExtension(ext); contentType != "" {
			return contentType
		}
	}
	return "application/octet-stream"
}
