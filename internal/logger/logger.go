package logger

import (
	"io"
	"log"
	"os"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

type Logger interface {
	Debug(format string, v ...interface{})
	Info(format string, v ...interface{})
	Warn(format string, v ...interface{})
	Error(format string, v ...interface{})
}

type logger struct {
	level  Level
	out    io.Writer
	logger *log.Logger
}

func New(level Level, out io.Writer) Logger {
	return &logger{
		level:  level,
		out:    out,
		logger: log.New(out, "", log.LstdFlags),
	}
}

func (l *logger) Debug(format string, v ...interface{}) {
	if l.level <= DEBUG {
		l.logger.Printf("[DEBUG] "+format, v...)
	}
}

func (l *logger) Info(format string, v ...interface{}) {
	if l.level <= INFO {
		l.logger.Printf("[INFO] "+format, v...)
	}
}

func (l *logger) Warn(format string, v ...interface{}) {
	if l.level <= WARN {
		l.logger.Printf("[WARN] "+format, v...)
	}
}

func (l *logger) Error(format string, v ...interface{}) {
	if l.level <= ERROR {
		l.logger.Printf("[ERROR] "+format, v...)
	}
}

// ParseLevel 解析日志级别字符串
func ParseLevel(level string) Level {
	switch level {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn":
		return WARN
	case "error":
		return ERROR
	default:
		return INFO
	}
}

// GetLogger 获取全局日志记录器
func GetLogger() Logger {
	level := ParseLevel("info") // 默认级别
	return New(level, os.Stdout)
}
