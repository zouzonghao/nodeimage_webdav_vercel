// package logger 提供了一个灵活的日志记录系统，支持不同的日志级别和输出目标，
// 包括一个特殊的 WebSocket logger，用于将日志实时推送到前端。
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"nodeimage_webdav_webui/pkg/websocket"
)

// LogLevel 定义了日志的严重性级别。
type LogLevel int

const (
	DEBUG LogLevel = iota // 最详细的日志，用于开发调试
	INFO                  // 常规信息性消息
	WARN                  // 警告信息，表示可能存在的问题
	ERROR                 // 错误信息，表示操作失败
)

// StringToLogLevel 将字符串（如 "info", "debug"）转换为对应的 LogLevel 枚举值。
// 这是为了方便从配置文件或环境变量中读取日志级别。
func StringToLogLevel(levelStr string) LogLevel {
	switch strings.ToLower(levelStr) {
	case "debug":
		return DEBUG
	case "warn":
		return WARN
	case "error":
		return ERROR
	default:
		return INFO
	}
}

// Logger 是所有 logger 实现都必须遵循的接口。
type Logger interface {
	Debug(format string, v ...interface{})
	Info(format string, v ...interface{})
	Warn(format string, v ...interface{})
	Error(format string, v ...interface{})
}

// --- Standard Logger ---

// loggerImpl 是标准的 Logger 实现，将日志输出到指定的 io.Writer (例如 os.Stdout)。
type loggerImpl struct {
	level  LogLevel    // 此 logger 将忽略低于此级别的所有消息
	logger *log.Logger // Go 标准库的 logger 实例
}

// New 创建一个新的标准 logger 实例。
func New(level LogLevel, out io.Writer) Logger {
	return &loggerImpl{
		level:  level,
		logger: log.New(out, "", log.LstdFlags),
	}
}

// NewDefault 创建一个默认的 logger，级别为 INFO，输出到标准输出。
func NewDefault() Logger {
	return New(INFO, os.Stdout)
}

func (l *loggerImpl) Debug(format string, v ...interface{}) {
	if l.level <= DEBUG {
		l.logger.Printf("[DEBUG] "+format, v...)
	}
}

func (l *loggerImpl) Info(format string, v ...interface{}) {
	if l.level <= INFO {
		l.logger.Printf("[INFO] "+format, v...)
	}
}

func (l *loggerImpl) Warn(format string, v ...interface{}) {
	if l.level <= WARN {
		l.logger.Printf("[WARN] "+format, v...)
	}
}

func (l *loggerImpl) Error(format string, v ...interface{}) {
	if l.level <= ERROR {
		l.logger.Printf("[ERROR] "+format, v...)
	}
}

// --- Websocket Logger ---

// websocketLogger 是一个特殊的 logger 实现，它将日志消息“装饰”后，
// 通过 WebSocket 推送到前端，并同时将其传递给另一个“备用”logger（通常是标准 logger）。
type websocketLogger struct {
	hub      *websocket.Hub // WebSocket 管理器，用于广播消息
	fallback Logger         // 备用 logger，用于将消息也输出到控制台
	level    LogLevel       // 此 logger 的日志级别
}

// NewWebsocketLogger 创建一个新的 websocketLogger 实例。
func NewWebsocketLogger(hub *websocket.Hub, fallback Logger, level LogLevel) Logger {
	return &websocketLogger{
		hub:      hub,
		fallback: fallback,
		level:    level,
	}
}

// log 是内部的通用日志处理方法。
func (l *websocketLogger) log(level LogLevel, levelStr string, format string, v ...interface{}) {
	// 如果消息的级别低于此 logger 的设定级别，则忽略
	if l.level > level {
		return
	}

	msg := fmt.Sprintf(format, v...)
	// 将日志格式化为带样式的 HTML，以便在前端美观地显示
	timestamp := time.Now().Format("15:04:05")
	htmlMsg := fmt.Sprintf(`<span class="log-time">[%s]</span> <span class="log-%s">[%s]</span> %s`, timestamp, levelStr, levelStr, msg)

	// 通过 WebSocket 广播格式化后的消息
	l.hub.Broadcast(websocket.Message{Type: "log", Content: htmlMsg})

	// 同时，将原始消息发送到备用 logger
	switch level {
	case DEBUG:
		l.fallback.Debug(msg)
	case INFO:
		l.fallback.Info(msg)
	case WARN:
		l.fallback.Warn(msg)
	case ERROR:
		l.fallback.Error(msg)
	}
}

func (l *websocketLogger) Debug(format string, v ...interface{}) {
	l.log(DEBUG, "DEBUG", format, v...)
}

func (l *websocketLogger) Info(format string, v ...interface{}) {
	l.log(INFO, "INFO", format, v...)
}

func (l *websocketLogger) Warn(format string, v ...interface{}) {
	l.log(WARN, "WARN", format, v...)
}

func (l *websocketLogger) Error(format string, v ...interface{}) {
	l.log(ERROR, "ERROR", format, v...)
}
