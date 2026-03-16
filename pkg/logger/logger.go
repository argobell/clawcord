package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

type LogLevel = zerolog.Level

const (
	DEBUG = zerolog.DebugLevel
	INFO  = zerolog.InfoLevel
	WARN  = zerolog.WarnLevel
	ERROR = zerolog.ErrorLevel
	FATAL = zerolog.FatalLevel
)

var (
	logLevelNames = map[LogLevel]string{
		DEBUG: "DEBUG",
		INFO:  "INFO",
		WARN:  "WARN",
		ERROR: "ERROR",
		FATAL: "FATAL",
	}

	currentLevel = INFO
	logger       zerolog.Logger
	fileLogger   zerolog.Logger
	logFile      *os.File
	once         sync.Once
	mu           sync.RWMutex
)

func init() {
	// once.Do 确保日志记录器只初始化一次，避免重复设置全局日志级别和创建日志记录器
	once.Do(func() {
		// 设置全局日志级别
		zerolog.SetGlobalLevel(zerolog.InfoLevel)

		// 创建控制台输出的日志记录器，使用自定义格式化函数处理多行字符串和JSON对象
		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",

			FormatFieldValue: formatFieldValue,
		}

		// New 创建一个新的日志记录器，使用控制台输出
		// Logger() 返回一个新的 Logger 实例
		// With() 方法用于添加上下文字段，这里添加了时间戳字段
		// Timestamp() 方法指定时间戳的格式
		logger = zerolog.New(consoleWriter).With().Timestamp().Logger()
		// 初始化文件日志记录器，默认输出到标准输出，后续可以通过 SetLogFile 设置日志文件
		fileLogger = zerolog.Logger{}
	})
}

func formatFieldValue(i any) string {
	var s string

	// 处理不同类型的输入
	// 字符串和字节切片直接转换为字符串，其他类型使用 fmt.Sprintf 格式化为字符串
	switch val := i.(type) {
	case string:
		s = val
	case []byte:
		s = string(val)
	default:
		return fmt.Sprintf("%v", i)
	}

	// 尝试去除字符串的引号，如果成功则使用去除引号后的字符串
	if unquoted, err := strconv.Unquote(s); err == nil {
		s = unquoted
	}

	// 如果字符串包含换行符，则在前面添加一个换行符，以便在日志输出中更清晰地显示多行内容
	if strings.Contains(s, "\n") {
		return fmt.Sprintf("\n%s", s)
	}

	// 如果字符串包含空格，但不是 JSON 对象或数组，则将其用引号括起来，以确保在日志输出中正确显示
	if strings.Contains(s, " ") {
		if (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
			(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) {
			return s
		}
		return fmt.Sprintf("%q", s)
	}

	return s
}

// SetLevel 设置日志级别，使用互斥锁保护对 currentLevel 的访问，确保线程安全
func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = level
	zerolog.SetGlobalLevel(level)
}

// GetLevel 获取当前日志级别，使用读锁保护对 currentLevel 的访问，确保线程安全
func GetLevel() LogLevel {
	mu.RLock()
	defer mu.RUnlock()
	return currentLevel
}

func EnableFileLogging(filePath string) error {
	mu.Lock()
	defer mu.Unlock()

	// MkdirAll() 创建日志目录（如果不存在）
	// Dir() 获取文件路径的目录部分
	// 755 权限表示所有者有读、写和执行权限，组用户和其他用户有读和执行权限
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// 打开日志文件，使用 os.O_CREATE 创建文件（如果不存在），os.O_WRONLY 以写入模式打开文件，os.O_APPEND 在文件末尾追加日志
	// 644 权限表示所有者有读和写权限，组用户和其他用户有读权限
	newFile, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// 关闭之前的日志文件（如果存在）
	if logFile != nil {
		logFile.Close()
	}

	// 创建新的文件日志记录器，使用新打开的日志文件作为输出
	logFile = newFile
	// New 创建一个新的日志记录器，使用文件输出
	// Claller() 方法添加调用者信息（文件名和行号）到日志中
	fileLogger = zerolog.New(logFile).With().Timestamp().Caller().Logger()
	return nil
}

// DisableFileLogging 禁用文件日志记录，关闭日志文件并重置文件日志记录器
func DisableFileLogging() {
	mu.Lock()
	defer mu.Unlock()

	// 关闭日志文件并将 logFile 设置为 nil，确保资源得到正确释放
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	// 重置文件日志记录器，确保不再写入日志文件
	fileLogger = zerolog.Logger{}
}

// getCallerInfo 获取调用者信息，返回文件名、行号和函数名
func getCallerInfo() (string, int, string) {
	// runtime.Caller() 函数用于获取调用者的信息
	// 参数 i 表示要获取的调用者层级，0 表示当前函数，1 表示调用当前函数的函数，以此类推
	// 循环从 2 开始，跳过当前函数和直接调用者，继续向上查找调用者信息
	// 直到找到一个非日志相关的调用者或者达到最大层级（15）
	for i := 2; i < 15; i++ {
		// runtime.Caller() 返回调用者的程序计数器（PC）、文件名、行号和一个布尔值，表示是否成功获取调用者信息
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			continue
		}

		// runtime.FuncForPC() 函数根据程序计数器获取调用者的函数信息，返回一个 *runtime.Func 类型的对象
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}

		// 过滤掉日志相关的调用者，避免在日志输出中显示日志库的内部调用信息
		if strings.HasSuffix(file, "/logger.go") {
			continue
		}

		// 获取调用者的函数名，并过滤掉以 "runtime." 开头的函数，避免显示运行时库的调用信息
		funcName := fn.Name()
		if strings.HasPrefix(funcName, "runtime.") {
			continue
		}

		// 返回调用者的文件名（使用 filepath.Base 获取文件名的最后一部分）、行号和函数名
		return filepath.Base(file), line, filepath.Base(funcName)
	}

	return "???", 0, "???"
}

//nolint:zerologlint
func getEvent(logger zerolog.Logger, level LogLevel) *zerolog.Event {
	switch level {
	case zerolog.DebugLevel:
		return logger.Debug()
	case zerolog.InfoLevel:
		return logger.Info()
	case zerolog.WarnLevel:
		return logger.Warn()
	case zerolog.ErrorLevel:
		return logger.Error()
	case zerolog.FatalLevel:
		return logger.Fatal()
	default:
		return logger.Info()
	}
}

// appendFields 将字段添加到日志事件中
// 使用 type switch 根据字段值的类型调用相应的方法添加字段
func appendFields(event *zerolog.Event, fields map[string]any) {
	for k, v := range fields {
		// 对于字符串类型，直接使用 Str() 方法添加字段，避免将字符串再次序列化为 JSON 格式
		switch val := v.(type) {
		case string:
			event.Str(k, val)
		case int:
			event.Int(k, val)
		case int64:
			event.Int64(k, val)
		case float64:
			event.Float64(k, val)
		case bool:
			event.Bool(k, val)
		default:
			// 其他类型使用 Interface() 方法添加字段，允许 zerolog 根据实际类型进行处理
			// 例如 struct、slice 和 map 等复杂类型会被序列化为 JSON 格式
			event.Interface(k, v)
		}
	}
}

// logMessage 记录日志消息，根据日志级别、组件名称、消息内容和字段信息构建日志事件并输出
func logMessage(level LogLevel, component string, message string, fields map[string]any) {
	// 如果日志级别低于当前设置的日志级别，则直接返回，不记录日志
	if level < currentLevel {
		return
	}

	// 获取调用者信息，包括文件名、行号和函数名，用于在日志中显示调用者的位置信息
	callerFile, callerLine, callerFunc := getCallerInfo()

	// 根据日志级别获取对应的日志事件对象，使用 getEvent() 函数根据日志级别返回相应的日志事件对象
	event := getEvent(logger, level)

	if component != "" {
		// 如果组件名称不为空，则在日志事件中添加一个 "caller" 字段，格式为 "component file:line (function)"
		event.Str("caller", fmt.Sprintf("%-6s %s:%d (%s)", component, callerFile, callerLine, callerFunc))
	} else {
		// 如果组件名称为空，则在日志事件中添加一个 "caller" 字段，格式为 "<none> file:line (function)"
		event.Str("caller", fmt.Sprintf("<none> %s:%d (%s)", callerFile, callerLine, callerFunc))
	}

	// 将字段信息添加到日志事件中
	appendFields(event, fields)
	// 输出日志消息
	event.Msg(message)

	// 如果文件日志记录器的日志级别不为 NoLevel，则也将日志消息记录到文件中
	if fileLogger.GetLevel() != zerolog.NoLevel {
		fileEvent := getEvent(fileLogger, level)

		if component != "" {
			fileEvent.Str("component", component)
		}

		appendFields(fileEvent, fields)
		fileEvent.Msg(message)
	}

	if level == FATAL {
		os.Exit(1)
	}
}

func Debug(message string) {
	logMessage(DEBUG, "", message, nil)
}

func DebugC(component string, message string) {
	logMessage(DEBUG, component, message, nil)
}

func Debugf(message string, ss ...any) {
	logMessage(DEBUG, "", fmt.Sprintf(message, ss...), nil)
}

func DebugF(message string, fields map[string]any) {
	logMessage(DEBUG, "", message, fields)
}

func DebugCF(component string, message string, fields map[string]any) {
	logMessage(DEBUG, component, message, fields)
}

func Info(message string) {
	logMessage(INFO, "", message, nil)
}

func InfoC(component string, message string) {
	logMessage(INFO, component, message, nil)
}

func InfoF(message string, fields map[string]any) {
	logMessage(INFO, "", message, fields)
}

func Infof(message string, ss ...any) {
	logMessage(INFO, "", fmt.Sprintf(message, ss...), nil)
}

func InfoCF(component string, message string, fields map[string]any) {
	logMessage(INFO, component, message, fields)
}

func Warn(message string) {
	logMessage(WARN, "", message, nil)
}

func WarnC(component string, message string) {
	logMessage(WARN, component, message, nil)
}

func WarnF(message string, fields map[string]any) {
	logMessage(WARN, "", message, fields)
}

func WarnCF(component string, message string, fields map[string]any) {
	logMessage(WARN, component, message, fields)
}

func Error(message string) {
	logMessage(ERROR, "", message, nil)
}

func ErrorC(component string, message string) {
	logMessage(ERROR, component, message, nil)
}

func Errorf(message string, ss ...any) {
	logMessage(ERROR, "", fmt.Sprintf(message, ss...), nil)
}

func ErrorF(message string, fields map[string]any) {
	logMessage(ERROR, "", message, fields)
}

func ErrorCF(component string, message string, fields map[string]any) {
	logMessage(ERROR, component, message, fields)
}

func Fatal(message string) {
	logMessage(FATAL, "", message, nil)
}

func FatalC(component string, message string) {
	logMessage(FATAL, component, message, nil)
}

func Fatalf(message string, ss ...any) {
	logMessage(FATAL, "", fmt.Sprintf(message, ss...), nil)
}

func FatalF(message string, fields map[string]any) {
	logMessage(FATAL, "", message, fields)
}

func FatalCF(component string, message string, fields map[string]any) {
	logMessage(FATAL, component, message, fields)
}
