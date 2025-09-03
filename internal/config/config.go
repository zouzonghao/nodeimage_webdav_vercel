// package config 负责集中管理应用程序的所有配置项。
package config

import (
	"os"
	"strconv"
)

// Config 结构体聚合了应用程序的所有配置。
type Config struct {
	NodeImageCookie string // 用于全量同步
	NodeImageAPIKey string // 用于增量同步
	NodeImageAPIURL string // NodeImage Cookie API 的基础 URL
	WebdavURL       string
	WebdavUsername  string
	WebdavPassword  string
	WebdavBasePath  string // WebDAV 上的同步根目录
	SyncConcurrency int    // 同步操作的并发数
	SyncInterval    int    // 定时增量同步的间隔（分钟）
	LogLevel        string // 日志级别 (e.g., "info", "debug")
	Port            string // Web 服务器监听的端口
}

// LoadConfig 从环境变量加载配置，并应用默认值。
func LoadConfig() *Config {
	cfg := &Config{
		NodeImageCookie: os.Getenv("NODEIMAGE_COOKIE"),
		NodeImageAPIKey: os.Getenv("NODEIMAGE_API_KEY"),
		NodeImageAPIURL: getEnv("NODEIMAGE_API_URL", "https://api.nodeimage.com/api/images"),
		WebdavURL:       getEnv("WEBDAV_URL", "https://dav.jianguoyun.com/dav"),
		WebdavUsername:  os.Getenv("WEBDAV_USERNAME"),
		WebdavPassword:  os.Getenv("WEBDAV_PASSWORD"),
		WebdavBasePath:  os.Getenv("WEBDAV_FOLDER"),
		SyncConcurrency: getEnvAsInt("SYNC_CONCURRENCY", 5),
		SyncInterval:    getEnvAsInt("SYNC_INTERVAL", 0), // 0 表示禁用定时同步
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		Port:            getEnv("PORT", "37373"),
	}
	return cfg
}

// getEnv 是一个辅助函数，用于读取环境变量，如果为空则返回默认值。
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// getEnvAsInt 是一个辅助函数，用于将环境变量解析为整数，如果失败或未设置则返回默认值。
func getEnvAsInt(name string, fallback int) int {
	valueStr := getEnv(name, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return fallback
}
