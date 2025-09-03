package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"nodeimage_webdav_webui/internal/config"
	sync_lib "nodeimage_webdav_webui/internal/sync"
	"nodeimage_webdav_webui/pkg/logger"
	"nodeimage_webdav_webui/pkg/stats"
	"nodeimage_webdav_webui/pkg/websocket"

	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
)

var (
	appConfig   *config.Config
	configMutex sync.RWMutex
	hub         *websocket.Hub
	log         logger.Logger
	st          *stats.Stats
	syncMutex   sync.Mutex
	httpClient  *http.Client
	store       *sessions.CookieStore
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("警告：未找到 .env 文件，将依赖系统环境变量")
	}

	appConfig = config.LoadConfig()

	if appConfig.Password != "" {
		store = sessions.NewCookieStore([]byte("secret-key")) // 在生产环境中应使用更安全的密钥
	}

	logLevel := logger.StringToLogLevel(appConfig.LogLevel)
	log = logger.New(logLevel, os.Stdout)
	st = stats.New()
	hub = websocket.NewHub()
	go hub.Run()

	httpClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 30 * time.Second,
	}

	if appConfig.SyncInterval > 0 {
		log.Info("已设置定时同步，每 %d 分钟执行一次增量同步", appConfig.SyncInterval)
		ticker := time.NewTicker(time.Duration(appConfig.SyncInterval) * time.Minute)
		go func() {
			safeGo := func(isFull bool) {
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Error("捕获到未处理的 panic: %v", r)
						}
					}()
					runSync(isFull, httpClient)
				}()
			}
			safeGo(false)
			for range ticker.C {
				safeGo(false)
			}
		}()
	}

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./public"))
	mux.Handle("/", authMiddleware(fs))

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		websocket.ServeWs(hub, w, r)
	})
	mux.HandleFunc("/login", loginHandler)
	mux.Handle("/api/sync", authMiddleware(http.HandlerFunc(syncHandler)))
	mux.Handle("/api/config", authMiddleware(http.HandlerFunc(configHandler)))
	mux.HandleFunc("/api/check-auth", checkAuthHandler)

	log.Info("服务器启动，监听端口: %s", appConfig.Port)
	if err := http.ListenAndServe(":"+appConfig.Port, mux); err != nil {
		log.Error("服务器启动失败: %v", err)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if appConfig.Password == "" {
		http.Error(w, "未设置密码，无需登录", http.StatusBadRequest)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST 方法", http.StatusMethodNotAllowed)
		return
	}

	var creds struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "无效的请求", http.StatusBadRequest)
		return
	}

	if creds.Password != appConfig.Password {
		http.Error(w, "密码错误", http.StatusUnauthorized)
		return
	}

	session, _ := store.Get(r, "session-name")
	session.Values["authenticated"] = true
	err := session.Save(r, w)
	if err != nil {
		log.Error("保存 session 失败: %v", err)
		http.Error(w, "无法保存 session", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func checkAuthHandler(w http.ResponseWriter, r *http.Request) {
	if appConfig.Password == "" {
		json.NewEncoder(w).Encode(map[string]bool{"authenticated": true})
		return
	}

	session, _ := store.Get(r, "session-name")
	auth, ok := session.Values["authenticated"].(bool)
	if !ok || !auth {
		json.NewEncoder(w).Encode(map[string]bool{"authenticated": false})
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"authenticated": true})
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if appConfig.Password == "" {
			next.ServeHTTP(w, r)
			return
		}

		// 允许访问登录页面和其中的静态资源
		if r.URL.Path == "/login.html" {
			http.ServeFile(w, r, "./public/login.html")
			return
		}

		session, _ := store.Get(r, "session-name")
		auth, ok := session.Values["authenticated"].(bool)

		if !ok || !auth {
			// 如果是 API 请求，返回 401
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "未授权", http.StatusUnauthorized)
				return
			}
			// 否则重定向到登录页
			http.Redirect(w, r, "/login.html", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func syncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST 方法", http.StatusMethodNotAllowed)
		return
	}
	mode := r.URL.Query().Get("mode")
	isFullSync := mode == "full"

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("捕获到未处理的 panic: %v", r)
			}
		}()
		runSync(isFullSync, httpClient)
	}()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("同步任务已启动..."))
}

func configHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.Lock()
	defer configMutex.Unlock()

	switch r.Method {
	case http.MethodGet:
		response := map[string]bool{
			"isCookieSet": appConfig.NodeImageCookie != "",
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
		appConfig.NodeImageCookie = payload.Cookie
		log.Info("NodeImage Cookie 已通过 API 更新")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Cookie 更新成功"))

	default:
		http.Error(w, "不支持的方法", http.StatusMethodNotAllowed)
	}
}

func runSync(isFullSync bool, httpClient *http.Client) {
	if !syncMutex.TryLock() {
		log.Warn("同步任务已在运行中，本次请求被跳过")
		wsLogger := logger.NewWebsocketLogger(hub, log, logger.StringToLogLevel(appConfig.LogLevel))
		wsLogger.Warn("同步任务已在运行中，本次请求被跳过")
		return
	}
	defer syncMutex.Unlock()

	wsLogger := logger.NewWebsocketLogger(hub, log, logger.StringToLogLevel(appConfig.LogLevel))
	wsLogger.Info("")
	hub.Broadcast(websocket.Message{Type: "syncStatus", Content: "syncing"})

	configMutex.RLock()
	activeConfig := *appConfig
	configMutex.RUnlock()

	syncConfig := sync_lib.Config{
		NodeImageCookie: activeConfig.NodeImageCookie,
		NodeImageAPIKey: activeConfig.NodeImageAPIKey,
		NodeImageAPIURL: activeConfig.NodeImageAPIURL,
		WebdavURL:       activeConfig.WebdavURL,
		WebdavUsername:  activeConfig.WebdavUsername,
		WebdavPassword:  activeConfig.WebdavPassword,
		WebdavBasePath:  activeConfig.WebdavBasePath,
		SyncConcurrency: activeConfig.SyncConcurrency,
	}

	result := sync_lib.RunSync(context.Background(), wsLogger, syncConfig, isFullSync, httpClient)

	resultJSON, _ := json.Marshal(result)
	hub.Broadcast(websocket.Message{Type: "syncResult", Content: string(resultJSON)})
	hub.Broadcast(websocket.Message{Type: "syncStatus", Content: "idle"})
}
