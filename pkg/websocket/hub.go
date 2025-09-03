// package websocket 提供了管理 WebSocket 连接和消息广播的功能。
package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"time"

	"github.com/gorilla/websocket"
)

const (
	// 等待 pong 消息的最大时间。
	pongWait = 60 * time.Second
	// 发送 ping 消息到对端的周期。必须小于 pongWait。
	pingPeriod = (pongWait * 9) / 10
	// 写消息到对端的最大等待时间。
	writeWait = 10 * time.Second
)

// upgrader 将标准的 HTTP 连接升级为 WebSocket 连接。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Message 定义了在 WebSocket 上传输的消息结构。
type Message struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// Client 是 Hub 和 websocket 连接之间的中间人。
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Hub 负责管理所有的 WebSocket 客户端连接。
type Hub struct {
	clients    map[*Client]bool // 存储所有活跃的客户端连接
	broadcast  chan []byte      // 用于广播消息的通道
	register   chan *Client     // 注册新连接的通道
	unregister chan *Client     // 注销断开连接的通道
	mutex      sync.Mutex       // 保护对 clients map 的并发访问
}

// NewHub 创建并返回一个新的 Hub 实例。
func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

// readPump 将消息从 websocket 连接泵送到 hub。
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		// 在这个应用中，我们忽略从客户端收到的消息
		if _, _, err := c.conn.ReadMessage(); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
	}
}

// writePump 将消息从 hub 泵送到 websocket 连接。
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// hub 关闭了通道。
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Run 启动 Hub 的主循环。
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()
			log.Println("WebSocket client registered")
		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Println("WebSocket client unregistered")
			}
			h.mutex.Unlock()
		case message := <-h.broadcast:
			h.mutex.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mutex.Unlock()
		}
	}
}

// Broadcast 广播消息。
func (h *Hub) Broadcast(message Message) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal broadcast message: %v", err)
		return
	}
	h.broadcast <- data
}

// ServeWs 处理来自对端的 websocket 请求。
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}
