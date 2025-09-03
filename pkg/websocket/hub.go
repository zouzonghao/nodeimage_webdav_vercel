// package websocket 提供了管理 WebSocket 连接和消息广播的功能。
package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// upgrader 将标准的 HTTP 连接升级为 WebSocket 连接。
// CheckOrigin 设置为 true 以允许所有来源的连接，方便开发。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Message 定义了在 WebSocket 上传输的消息结构。
// Type 用于让前端区分不同类型的消息（如日志、结果、状态）。
type Message struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// Hub 负责管理所有的 WebSocket 客户端连接。
type Hub struct {
	clients    map[*websocket.Conn]bool // 存储所有活跃的客户端连接
	broadcast  chan []byte              // 用于广播消息的通道
	register   chan *websocket.Conn     // 注册新连接的通道
	unregister chan *websocket.Conn     // 注销断开连接的通道
	mutex      sync.Mutex               // 保护对 clients map 的并发访问
}

// NewHub 创建并返回一个新的 Hub 实例。
func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		clients:    make(map[*websocket.Conn]bool),
	}
}

// Run 启动 Hub 的主循环，监听并处理来自各个通道的事件。
// 这是一个阻塞方法，应该在一个单独的 Goroutine 中运行。
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			// 处理新客户端注册
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()
			log.Println("WebSocket client registered")
		case client := <-h.unregister:
			// 处理客户端断开连接
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
				log.Println("WebSocket client unregistered")
			}
			h.mutex.Unlock()
		case message := <-h.broadcast:
			// 处理消息广播
			h.mutex.Lock()
			// 向所有已注册的客户端发送消息
			for client := range h.clients {
				err := client.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					log.Printf("WebSocket write error: %v", err)
					// 如果写入失败，认为客户端已断开，将其放入注销通道
					go func(c *websocket.Conn) {
						h.unregister <- c
					}(client)
				}
			}
			h.mutex.Unlock()
		}
	}
}

// Broadcast 将一个 Message 序列化为 JSON 并放入广播通道，由 Run 方法处理发送。
func (h *Hub) Broadcast(message Message) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal broadcast message: %v", err)
		return
	}
	h.broadcast <- data
}

// ServeWs 是一个 HTTP handler，用于处理 WebSocket 的升级请求。
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	// 将新建立的连接注册到 Hub
	hub.register <- conn

	// 启动一个 Goroutine 来读取来自客户端的消息。
	// 在这个应用中，我们不处理客户端发来的消息，但这可以防止连接因超时而断开。
	// 当读取失败时（通常是客户端关闭了连接），就将此客户端注销。
	go func() {
		defer func() {
			hub.unregister <- conn
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}
