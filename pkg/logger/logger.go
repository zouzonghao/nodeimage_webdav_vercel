package logger

import (
	"io"
	"log"
	"os"
)

// LogLevel 定义了日志级别。
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger 是一个简单的日志记录器接口。
type Logger interface {
	Debug(format string, v ...interface{})
	Info(format string, v ...interface{})
	Warn(format string, v ...interface{})
	Error(format string, v ...interface{})
}

// loggerImpl 是 Logger 接口的实现。
type loggerImpl struct {
	level  LogLevel
	logger *log.Logger
}

// New 创建一个新的 Logger 实例。
func New(level LogLevel, out io.Writer) Logger {
	return &loggerImpl{
		level:  level,
		logger: log.New(out, "", log.LstdFlags),
	}
}

// NewDefault 创建一个默认的 Logger，输出到 os.Stdout，级别为 INFO。
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

// Printf 允许 loggerImpl 直接用作 log.Printf
func (l *loggerImpl) Printf(format string, v ...interface{}) {
	l.logger.Printf(format, v...)
}
