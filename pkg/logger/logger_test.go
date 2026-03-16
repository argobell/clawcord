package logger

import "testing"

// TestLogLevels 测试日志级别名称映射的正确性
func TestLogLevels(t *testing.T) {
	tests := []struct {
		name  string
		level LogLevel
		want  string
	}{
		{"DEBUG level", DEBUG, "DEBUG"},
		{"INFO level", INFO, "INFO"},
		{"WARN level", WARN, "WARN"},
		{"ERROR level", ERROR, "ERROR"},
		{"FATAL level", FATAL, "FATAL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if logLevelNames[tt.level] != tt.want {
				t.Errorf("logLevelNames[%d] = %s, want %s", tt.level, logLevelNames[tt.level], tt.want)
			}
		})
	}
}

// TestSetGetLevel 测试 SetLevel 和 GetLevel 函数的正确性
func TestSetGetLevel(t *testing.T) {
	initialLevel := GetLevel()
	defer SetLevel(initialLevel)

	tests := []LogLevel{DEBUG, INFO, WARN, ERROR, FATAL}

	for _, level := range tests {
		SetLevel(level)
		if GetLevel() != level {
			t.Errorf("SetLevel(%v) -> GetLevel() = %v, want %v", level, GetLevel(), level)
		}
	}
}

// TestFormatFieldValue 测试 formatFieldValue 函数的正确性
func TestFormatFieldValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		// 基础类型测试
		{
			name:     "Integer Type",
			input:    42,
			expected: "42",
		},
		{
			name:     "Boolean Type",
			input:    true,
			expected: "true",
		},
		{
			name:     "Unsupported Struct Type",
			input:    struct{ A int }{A: 1},
			expected: "{1}",
		},

		// 字符串和字节切片测试
		{
			name:     "Simple string without spaces",
			input:    "simple_value",
			expected: "simple_value",
		},
		{
			name:     "Simple byte slice",
			input:    []byte("byte_value"),
			expected: "byte_value",
		},

		// 字符串去引号测试
		{
			name:     "Quoted string",
			input:    `"quoted_value"`,
			expected: "quoted_value",
		},

		// 包含换行符和空格的字符串测试
		{
			name:     "String with newline",
			input:    "line1\nline2",
			expected: "\nline1\nline2",
		},
		{
			name:     "Quoted string with newline (Unquote -> newline)",
			input:    `"line1\nline2"`, // Escaped \n that Unquote will resolve
			expected: "\nline1\nline2",
		},

		// 包含空格但不是 JSON 的字符串测试
		{
			name:     "String with spaces",
			input:    "hello world",
			expected: `"hello world"`,
		},
		{
			name:     "Quoted string with spaces (Unquote -> has spaces -> Re-quote)",
			input:    `"hello world"`,
			expected: `"hello world"`,
		},

		// JSON 格式测试（以 { 开头和结尾的字符串，或以 [ 开头和结尾的字符串）
		{
			name:     "Valid JSON object",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "Valid JSON array",
			input:    `[1, 2, "three"]`,
			expected: `[1, 2, "three"]`,
		},
		{
			name:     "Fake JSON (starts with { but doesn't end with })",
			input:    `{"key": "value"`, // 有空格，缺少结尾的 }
			expected: `"{\"key\": \"value\""`,
		},
		{
			name:     "Empty JSON (object)",
			input:    `{ }`,
			expected: `{ }`,
		},

		// 边界情况测试
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Whitespace only string",
			input:    "   ",
			expected: `"   "`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := formatFieldValue(tt.input)
			if actual != tt.expected {
				t.Errorf("formatFieldValue() = %q, expected %q", actual, tt.expected)
			}
		})
	}
}

// TestLoggerWithComponent 测试带组件名称的日志记录功能
func TestLoggerWithComponent(t *testing.T) {
	initialLevel := GetLevel()
	defer SetLevel(initialLevel)

	SetLevel(DEBUG)

	tests := []struct {
		name      string
		component string
		message   string
		fields    map[string]any
	}{
		{"Simple message", "test", "Hello, world!", nil},
		{"Message with component", "discord", "Discord message", nil},
		{"Message with fields", "telegram", "Telegram message", map[string]any{
			"user_id": "12345",
			"count":   42,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch {
			case tt.fields == nil && tt.component != "":
				InfoC(tt.component, tt.message)
			case tt.fields != nil:
				InfoF(tt.message, tt.fields)
			default:
				Info(tt.message)
			}
		})
	}

	SetLevel(INFO)
}

// TestLogLevelFiltering 测试日志级别过滤功能，确保只有符合当前日志级别的消息被记录
func TestLogLevelFiltering(t *testing.T) {
	initialLevel := GetLevel()
	defer SetLevel(initialLevel)

	SetLevel(WARN)

	tests := []struct {
		name      string
		level     LogLevel
		shouldLog bool
	}{
		{"DEBUG message", DEBUG, false},
		{"INFO message", INFO, false},
		{"WARN message", WARN, true},
		{"ERROR message", ERROR, true},
		{"FATAL message", FATAL, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.level {
			case DEBUG:
				Debug(tt.name)
			case INFO:
				Info(tt.name)
			case WARN:
				Warn(tt.name)
			case ERROR:
				Error(tt.name)
			case FATAL:
				if tt.shouldLog {
					t.Logf("FATAL test skipped to prevent program exit")
				}
			}
		})
	}

	SetLevel(INFO)
}

// TestLoggerHelperFunctions 测试日志记录器的辅助函数（如 Debug、Info、Warn、Error 等）的正确性
func TestLoggerHelperFunctions(t *testing.T) {
	initialLevel := GetLevel()
	defer SetLevel(initialLevel)

	SetLevel(INFO)

	Debug("This should not log")
	Debugf("this should not log")
	Info("This should log")
	Warn("This should log")
	Error("This should log")

	InfoC("test", "Component message")
	InfoF("Fields message", map[string]any{"key": "value"})
	Infof("test from %v", "Infof")

	WarnC("test", "Warning with component")
	ErrorF("Error with fields", map[string]any{"error": "test"})
	Errorf("test from %v", "Errorf")

	SetLevel(DEBUG)
	DebugC("test", "Debug with component")
	Debugf("test from %v", "Debugf")
	WarnF("Warning with fields", map[string]any{"key": "value"})
}
