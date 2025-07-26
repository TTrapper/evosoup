package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub
	appState *AppState

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte
}

// readPump pumps messages from the websocket connection to the hub.
// In this high-throughput application, we don't need a read deadline.
// A broken connection will be detected by a write failure in the writePump.
func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)

		// Handle incoming messages from the client.
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		// We only expect JSON messages for controls.
		var msg UIMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("error unmarshalling message: %v", err)
			continue
		}

		// Route the message based on its type.
		switch msg.Type {
		case "set_jump_rate":
			log.Printf("Received set_jump_rate: %f", msg.Value)
			// Safely send to the hub's channel.
			select {
			case c.hub.SetJumpInterval <- msg.Value:
			default:
				log.Println("Jump rate channel is full, dropping message.")
			}
		case "command":
			log.Printf("Received command: %s", msg.Command)
			switch msg.Command {
			case "pause":
				c.appState.Pause()
			case "resume":
				c.appState.Resume()
			case "step":
				c.appState.Step()
			default:
				log.Printf("Unknown command received: %s", msg.Command)
			}
		case "set_view_start_index":
			log.Printf("Received set_view_start_index: %d", int(msg.Value))
			c.appState.SetViewStartIndex(int(msg.Value))
		case "set_32_bit_addressing":
			log.Printf("Received set_32_bit_addressing: %t", msg.Value == 1)
			c.appState.Set32BitAddressing(msg.Value == 1)
		case "set_relative_addressing":
			log.Printf("Received set_relative_addressing: %t", msg.Value == 1)
			c.appState.SetRelativeAddressing(msg.Value == 1)
		case "set_tracking_enabled":
			log.Printf("Received set_tracking_enabled: %t", msg.Value == 1)
			c.appState.SetTrackingEnabled(msg.Value == 1)
		case "set_ip_ptr":
			log.Printf("Received set_ip_ptr for IP %d to %d", msg.ID, msg.Ptr)
			c.appState.SetIPPtr(msg.ID, msg.Ptr)
		default:
			log.Printf("Unknown message type received: %s", msg.Type)
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
// A goroutine running writePump is started for each connection. This is the
// only place that should write to the connection.
func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			// Set a deadline on the write. If the write blocks for too long,
			// we assume the connection is dead.
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			var msgType int
			if json.Valid(message) {
				msgType = websocket.TextMessage
			} else {
				msgType = websocket.BinaryMessage
			}
			if err := c.conn.WriteMessage(msgType, message); err != nil {
				// An error writing the message (like a timeout) indicates a
				// broken connection.
				log.Printf("write error, closing connection: %v", err)
				return
			}
		}
	}
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	clients     map[*Client]bool
	Broadcast   chan []byte
	Register    chan *Client
	Unregister  chan *Client
	SetJumpInterval chan float64
	Pause       chan bool
}

// UIMessage defines the structure for incoming JSON messages from the UI.
type UIMessage struct {
	Type    string  `json:"type"`
	Value   float64 `json:"value"`
	Command string  `json:"command"`
	ID      int     `json:"id"`  // For IP tracking commands
	Ptr     int32   `json:"ptr"` // For setting IP pointer
}


// NewHub creates a new Hub object.
func NewHub() *Hub {
	return &Hub{
		Broadcast:   make(chan []byte, 256),
		Register:    make(chan *Client),
		Unregister:  make(chan *Client),
		clients:     make(map[*Client]bool),
		SetJumpInterval: make(chan float64, 8),
		Pause:       make(chan bool, 8),
	}
}

// Run starts the Hub's message-handling loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.clients[client] = true
		case client := <-h.Unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.Broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// The client's send buffer is full. Instead of disconnecting,
					// we just drop the message. The client will experience a
					// stutter, but won't get stuck. A truly dead connection
					// will be caught by the writePump's deadline.
				}
			}
		}
	}
}

// handleWebSocket upgrades HTTP connections to WebSocket connections and creates a Client.
func handleWebSocket(hub *Hub, appState *AppState, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	client := &Client{hub: hub, appState: appState, conn: conn, send: make(chan []byte, 256)}
	client.hub.Register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}

// serveIndex serves the main HTML file.
func serveIndex(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat("index.html"); os.IsNotExist(err) {
		http.Error(w, "index.html not found", http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, "index.html")
}

// StartServer initializes HTTP routes and starts the web server.
func StartServer(hub *Hub, appState *AppState) {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, appState, w, r)
	})
	http.HandleFunc("/", serveIndex)

	log.Println("Starting web server on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe Error: ", err)
	}
}
