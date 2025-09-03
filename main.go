package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	sync_lib "nodeimage_webdav_webui/internal/sync"
	"nodeimage_webdav_webui/pkg/logger"
	"nodeimage_webdav_webui/pkg/stats"
	"nodeimage_webdav_webui/pkg/websocket"

	"github.com/joho/godotenv"
)

var (
	// baseSyncConfig 从 .env 初始化，作为基准配置
	baseSyncConfig sync_lib.Config
	// currentSyncConfig 用于实际操作，允许在运行时通过 API 修改 Cookie
	currentSyncConfig sync_lib.Config
	// configMutex 保护对 currentSyncConfig 的并发访问
	configMutex sync.RWMutex
	// hub 是 WebSocket 连接的管理器
	hub *websocket.Hub
	// log 是全局的备用日志记录器 (输出到控制台)
	log logger.Logger
	// logLevel 控制日志输出的级别
	logLevel logger.LogLevel
	// st 用于跟踪全局的统计信息
	st *stats.Stats
	// syncMutex 确保同一时间只有一个同步任务在运行
	syncMutex sync.Mutex
)

func main() {
	// 优先从 .env 文件加载环境变量
	err := godotenv.Load()
	if err != nil {
		fmt.Println("警告：未找到 .env 文件，将依赖系统环境变量")
	}

	// --- 初始化组件 ---
	logLevel = logger.StringToLogLevel(os.Getenv("LOG_LEVEL"))
	log = logger.New(logLevel, os.Stdout)
	st = stats.New()
	hub = websocket.NewHub()
	go hub.Run()

	// --- 读取并设置配置 ---
	baseSyncConfig = sync_lib.Config{
		NodeImageCookie: os.Getenv("NODEIMAGE_COOKIE"),
		NodeImageAPIKey: os.Getenv("NODEIMAGE_API_KEY"),
		NodeImageAPIURL: os.Getenv("NODEIMAGE_API_URL"),
		WebdavURL:       os.Getenv("WEBDAV_URL"),
		WebdavUsername:  os.Getenv("WEBDAV_USERNAME"),
		WebdavPassword:  os.Getenv("WEBDAV_PASSWORD"),
		WebdavBasePath:  os.Getenv("WEBDAV_FOLDER"),
	}
	currentSyncConfig = baseSyncConfig

	// --- 启动定时增量同步任务 ---
	syncIntervalStr := os.Getenv("SYNC_INTERVAL")
	if syncIntervalStr != "" {
		syncInterval, err := strconv.Atoi(syncIntervalStr)
		if err == nil && syncInterval > 0 {
			log.Info("已设置定时同步，每 %d 分钟执行一次增量同步", syncInterval)
			ticker := time.NewTicker(time.Duration(syncInterval) * time.Minute)
			go func() {
				// 首次启动时先执行一次，以确保数据最新
				go runSync(false)
				// 然后按设定的间隔持续执行
				for range ticker.C {
					go runSync(false)
				}
			}()
		}
	}

	// --- 设置 HTTP 路由 ---
	// 静态文件服务 (HTML, CSS, JS)
	http.Handle("/", http.FileServer(http.Dir("./public")))
	// WebSocket 连接端点
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		websocket.ServeWs(hub, w, r)
	})
	// 手动触发同步的 API 端点
	http.HandleFunc("/api/sync", syncHandler)
	// 获取和更新配置的 API 端点
	http.HandleFunc("/api/config", configHandler)

	// --- 启动 Web 服务器 ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "37373"
	}
	log.Info("服务器启动，监听端口: %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Error("服务器启动失败: %v", err)
	}
}

// syncHandler 处理来自前端的手动同步请求
func syncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST 方法", http.StatusMethodNotAllowed)
		return
	}
	// 从 URL 查询参数判断是否为全量同步
	mode := r.URL.Query().Get("mode")
	isFullSync := mode == "full"

	// 在一个新的 Goroutine 中执行同步，避免阻塞 HTTP 请求
	go runSync(isFullSync)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("同步任务已启动..."))
}

// configHandler 处理前端对配置的获取和更新请求
func configHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.Lock()
	defer configMutex.Unlock()

	switch r.Method {
	case http.MethodGet:
		// 出于安全，只返回 Cookie 是否已设置的状态，不返回具体内容
		response := map[string]bool{
			"isCookieSet": currentSyncConfig.NodeImageCookie != "",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

	case http.MethodPost:
		var payload struct {
			Cookie string `json:"cookie"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "无效的请求体", http.StatusBadRequest)
			return
		}
		// 在内存中更新当前运行的 Cookie
		currentSyncConfig.NodeImageCookie = payload.Cookie
		log.Info("NodeImage Cookie 已通过 API 更新")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Cookie 更新成功"))

	default:
		http.Error(w, "不支持的方法", http.StatusMethodNotAllowed)
	}
}

// runSync 是执行同步的核心函数，带锁确保单实例运行
func runSync(isFullSync bool) {
	// 尝试获取锁，如果失败则说明已有任务在运行
	if !syncMutex.TryLock() {
		log.Warn("同步任务已在运行中，本次请求被跳过")
		wsLogger := logger.NewWebsocketLogger(hub, log, logLevel)
		wsLogger.Warn("同步任务已在运行中，本次请求被跳过")
		return
	}
	defer syncMutex.Unlock()

	// 创建一个特殊的 logger，它会同时向控制台和 WebSocket 输出日志
	wsLogger := logger.NewWebsocketLogger(hub, log, logLevel)

	// 在同步开始前输出一个空行作为分隔
	wsLogger.Info("")

	// 通过 WebSocket 通知前端同步开始
	hub.Broadcast(websocket.Message{Type: "syncStatus", Content: "syncing"})

	// 获取当前配置的只读副本以供本次同步使用
	configMutex.RLock()
	activeConfig := currentSyncConfig
	configMutex.RUnlock()

	// 执行同步逻辑
	result := sync_lib.RunSync(context.Background(), wsLogger, activeConfig, isFullSync)

	// 将详细的同步结果通过 WebSocket 发送给前端
	resultJSON, _ := json.Marshal(result)
	hub.Broadcast(websocket.Message{Type: "syncResult", Content: string(resultJSON)})

	// 通知前端同步结束
	hub.Broadcast(websocket.Message{Type: "syncStatus", Content: "idle"})
}
