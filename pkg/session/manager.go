package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/argobell/clawcord/pkg/providers"
)

// Session 结构体表示一个会话，包含会话的关键信息，例如消息列表、摘要、创建时间和更新时间。
type Session struct {
	Key      string              `json:"key"`               // 会话的唯一标识符
	Messages []providers.Message `json:"messages"`          // 会话中的消息列表
	Summary  string              `json:"summary,omitempty"` // 会话的摘要，可选字段
	Created  time.Time           `json:"created"`           // 会话的创建时间
	Updated  time.Time           `json:"updated"`           // 会话的最后更新时间
}

// SessionManager 结构体管理多个会话，提供线程安全的操作和持久化存储。
type SessionManager struct {
	sessions map[string]*Session // 内存中的会话存储，键为会话的唯一标识符
	mu       sync.RWMutex        // 读写锁，确保线程安全
	storage  string              // 会话数据的存储路径
}

// NewSessionManager 创建一个新的会话管理器，并加载存储中的会话数据。
func NewSessionManager(storage string) *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*Session),
		storage:  storage,
	}

	// 如果指定了存储路径，则创建目录并加载会话数据
	if storage != "" {
		os.MkdirAll(storage, 0o700)
		sm.loadSessions()
	}

	return sm
}

// GetOrCreate 获取指定键的会话，如果不存在则创建一个新的会话。
func (sm *SessionManager) GetOrCreate(key string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if ok {
		return session
	}

	// 创建新会话
	session = &Session{
		Key:      key,
		Messages: []providers.Message{},
		Created:  time.Now(),
		Updated:  time.Now(),
	}
	sm.sessions[key] = session

	return session
}

// AddMessage 向指定会话添加一条消息。
func (sm *SessionManager) AddMessage(sessionKey, role, content string) {
	sm.AddFullMessage(sessionKey, providers.Message{
		Role:    role,
		Content: content,
	})
}

// AddFullMessage 向指定会话添加完整的消息对象。
func (sm *SessionManager) AddFullMessage(sessionKey string, msg providers.Message) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[sessionKey]
	if !ok {
		// 如果会话不存在，则创建新会话
		session = &Session{
			Key:      sessionKey,
			Messages: []providers.Message{},
			Created:  time.Now(),
		}
		sm.sessions[sessionKey] = session
	}

	// 添加消息并更新会话的更新时间
	session.Messages = append(session.Messages, msg)
	session.Updated = time.Now()
}

// GetHistory 获取指定会话的消息历史。
func (sm *SessionManager) GetHistory(key string) []providers.Message {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, ok := sm.sessions[key]
	if !ok {
		return []providers.Message{}
	}

	// 返回消息的副本以避免外部修改
	history := make([]providers.Message, len(session.Messages))
	copy(history, session.Messages)
	return history
}

// GetSummary 获取指定会话的摘要。
func (sm *SessionManager) GetSummary(key string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, ok := sm.sessions[key]
	if !ok {
		return ""
	}
	return session.Summary
}

// SetSummary 设置指定会话的摘要。
func (sm *SessionManager) SetSummary(key string, summary string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if ok {
		session.Summary = summary
		session.Updated = time.Now()
	}
}

// TruncateHistory 截断指定会话的消息历史，仅保留最近的若干条消息。
func (sm *SessionManager) TruncateHistory(key string, keepLast int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if !ok {
		return
	}

	if keepLast <= 0 {
		// 如果保留条数小于等于 0，则清空消息历史
		session.Messages = []providers.Message{}
		session.Updated = time.Now()
		return
	}

	if len(session.Messages) <= keepLast {
		return
	}

	// 截断消息历史
	session.Messages = session.Messages[len(session.Messages)-keepLast:]
	session.Updated = time.Now()
}

// sanitizeFilename 清理文件名，将非法字符替换为下划线。
func sanitizeFilename(key string) string {
	s := strings.ReplaceAll(key, ":", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	return s
}

// Save 将指定会话保存到存储中。
func (sm *SessionManager) Save(key string) error {
	if sm.storage == "" {
		return nil
	}

	// 清理文件名
	filename := sanitizeFilename(key)

	if filename == "." || !filepath.IsLocal(filename) {
		return os.ErrInvalid
	}
	sm.mu.RLock()
	stored, ok := sm.sessions[key]
	if !ok {
		sm.mu.RUnlock()
		return nil
	}

	// 创建会话的快照
	snapshot := Session{
		Key:     stored.Key,
		Summary: stored.Summary,
		Created: stored.Created,
		Updated: stored.Updated,
	}
	if len(stored.Messages) > 0 {
		snapshot.Messages = make([]providers.Message, len(stored.Messages))
		copy(snapshot.Messages, stored.Messages)
	} else {
		snapshot.Messages = []providers.Message{}
	}
	sm.mu.RUnlock()

	// 序列化会话数据
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	sessionPath := filepath.Join(sm.storage, filename+".json")
	tmpFile, err := os.CreateTemp(sm.storage, "session-*.tmp")
	if err != nil {
		return err
	}

	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	// 写入数据到临时文件
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	// 将临时文件重命名为目标文件
	if err := os.Rename(tmpPath, sessionPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// loadSessions 从存储中加载所有会话数据。
func (sm *SessionManager) loadSessions() error {
	files, err := os.ReadDir(sm.storage)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		sessionPath := filepath.Join(sm.storage, file.Name())
		data, err := os.ReadFile(sessionPath)
		if err != nil {
			continue
		}

		var session Session
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}

		sm.sessions[session.Key] = &session
	}

	return nil
}

// Close 关闭会话管理器，目前未实现任何操作。
func (sm *SessionManager) Close() error {
	return nil
}

// SetHistory 设置指定会话的消息历史。
func (sm *SessionManager) SetHistory(key string, history []providers.Message) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if ok {
		msgs := make([]providers.Message, len(history))
		copy(msgs, history)
		session.Messages = msgs
		session.Updated = time.Now()
	}
}
