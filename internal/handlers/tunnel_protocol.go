package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// TunnelMessage represents a message in the tunnel protocol
type TunnelMessage struct {
	Type      string            `json:"type"`
	ID        string            `json:"id"`
	Method    string            `json:"method,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      []byte            `json:"body,omitempty"`
	Status    int               `json:"status,omitempty"`
	Error     string            `json:"error,omitempty"`
	Timestamp int64             `json:"timestamp"`
}

// TunnelProtocol handles the complete HTTP tunneling protocol
type TunnelProtocol struct {
	conn         *websocket.Conn
	tunnelID     string
	localPort    int
	pendingReqs  map[string]chan *TunnelMessage
	requestCount int64
}

func NewTunnelProtocol(conn *websocket.Conn, tunnelID string, localPort int) *TunnelProtocol {
	return &TunnelProtocol{
		conn:        conn,
		tunnelID:    tunnelID,
		localPort:   localPort,
		pendingReqs: make(map[string]chan *TunnelMessage),
	}
}

// HandleIncomingHTTPRequest processes an HTTP request and forwards it through the tunnel
func (tp *TunnelProtocol) HandleIncomingHTTPRequest(w http.ResponseWriter, r *http.Request) {
	tp.requestCount++
	requestID := fmt.Sprintf("%s-%d", tp.tunnelID, tp.requestCount)

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	r.Body.Close()

	// Convert headers to map
	headers := make(map[string]string)
	for name, values := range r.Header {
		headers[name] = strings.Join(values, ", ")
	}

	// Create tunnel message
	message := &TunnelMessage{
		Type:      "http_request",
		ID:        requestID,
		Method:    r.Method,
		URL:       r.URL.String(),
		Headers:   headers,
		Body:      body,
		Timestamp: time.Now().Unix(),
	}

	// Create response channel
	responseChan := make(chan *TunnelMessage, 1)
	tp.pendingReqs[requestID] = responseChan

	// Send request through tunnel
	if err := tp.sendMessage(message); err != nil {
		delete(tp.pendingReqs, requestID)
		http.Error(w, "Failed to send request through tunnel", http.StatusBadGateway)
		return
	}

	// Wait for response (with timeout)
	select {
	case response := <-responseChan:
		tp.writeHTTPResponse(w, response)
		delete(tp.pendingReqs, requestID)
	case <-time.After(30 * time.Second):
		delete(tp.pendingReqs, requestID)
		http.Error(w, "Tunnel request timeout", http.StatusGatewayTimeout)
	}
}

// HandleWebSocketUpgrade handles WebSocket upgrade requests through the tunnel
func (tp *TunnelProtocol) HandleWebSocketUpgrade(w http.ResponseWriter, r *http.Request) {
	tp.requestCount++
	requestID := fmt.Sprintf("%s-ws-%d", tp.tunnelID, tp.requestCount)

	// Convert headers to map
	headers := make(map[string]string)
	for name, values := range r.Header {
		headers[name] = strings.Join(values, ", ")
	}

	// Create WebSocket upgrade request
	message := &TunnelMessage{
		Type:      "websocket_upgrade",
		ID:        requestID,
		Method:    r.Method,
		URL:       r.URL.String(),
		Headers:   headers,
		Timestamp: time.Now().Unix(),
	}

	// Create response channel
	responseChan := make(chan *TunnelMessage, 1)
	tp.pendingReqs[requestID] = responseChan

	// Send upgrade request through tunnel
	if err := tp.sendMessage(message); err != nil {
		delete(tp.pendingReqs, requestID)
		http.Error(w, "Failed to send WebSocket upgrade through tunnel", http.StatusBadGateway)
		return
	}

	// Wait for upgrade response
	select {
	case response := <-responseChan:
		if response.Status == http.StatusSwitchingProtocols {
			tp.handleWebSocketTunnel(w, r, requestID)
		} else {
			tp.writeHTTPResponse(w, response)
		}
		delete(tp.pendingReqs, requestID)
	case <-time.After(10 * time.Second):
		delete(tp.pendingReqs, requestID)
		http.Error(w, "WebSocket upgrade timeout", http.StatusGatewayTimeout)
	}
}

// HandleTunnelMessage processes messages received from the agent
func (tp *TunnelProtocol) HandleTunnelMessage(messageBytes []byte) error {
	var message TunnelMessage
	if err := json.Unmarshal(messageBytes, &message); err != nil {
		return fmt.Errorf("failed to unmarshal tunnel message: %w", err)
	}

	switch message.Type {
	case "http_response":
		return tp.handleHTTPResponse(&message)
	case "websocket_upgrade_response":
		return tp.handleWebSocketUpgradeResponse(&message)
	case "websocket_data":
		return tp.handleWebSocketData(&message)
	case "ping":
		return tp.handlePing(&message)
	case "pong":
		return tp.handlePong(&message)
	default:
		log.Printf("Unknown tunnel message type: %s", message.Type)
	}

	return nil
}

func (tp *TunnelProtocol) handleHTTPResponse(message *TunnelMessage) error {
	if responseChan, exists := tp.pendingReqs[message.ID]; exists {
		select {
		case responseChan <- message:
		default:
			log.Printf("Response channel full for request %s", message.ID)
		}
	} else {
		log.Printf("No pending request found for ID: %s", message.ID)
	}
	return nil
}

func (tp *TunnelProtocol) handleWebSocketUpgradeResponse(message *TunnelMessage) error {
	if responseChan, exists := tp.pendingReqs[message.ID]; exists {
		select {
		case responseChan <- message:
		default:
			log.Printf("WebSocket upgrade response channel full for request %s", message.ID)
		}
	}
	return nil
}

func (tp *TunnelProtocol) handleWebSocketData(message *TunnelMessage) error {
	// Handle WebSocket data forwarding
	// This would be implemented based on the WebSocket connection mapping
	log.Printf("Received WebSocket data for ID: %s", message.ID)
	return nil
}

func (tp *TunnelProtocol) handlePing(message *TunnelMessage) error {
	// Respond with pong
	pongMessage := &TunnelMessage{
		Type:      "pong",
		ID:        message.ID,
		Timestamp: time.Now().Unix(),
	}
	return tp.sendMessage(pongMessage)
}

func (tp *TunnelProtocol) handlePong(message *TunnelMessage) error {
	// Update connection health
	log.Printf("Received pong from tunnel %s", tp.tunnelID)
	return nil
}

func (tp *TunnelProtocol) writeHTTPResponse(w http.ResponseWriter, response *TunnelMessage) {
	// Set status code
	if response.Status > 0 {
		w.WriteHeader(response.Status)
	}

	// Set headers
	for name, value := range response.Headers {
		w.Header().Set(name, value)
	}

	// Write body
	if len(response.Body) > 0 {
		w.Write(response.Body)
	}
}

func (tp *TunnelProtocol) handleWebSocketTunnel(w http.ResponseWriter, r *http.Request, requestID string) {
	// Upgrade the connection to WebSocket
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket connection: %v", err)
		return
	}
	defer wsConn.Close()

	// Handle WebSocket messages
	for {
		messageType, data, err := wsConn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			break
		}

		// Forward WebSocket message through tunnel
		tunnelMsg := &TunnelMessage{
			Type:      "websocket_data",
			ID:        requestID,
			Body:      data,
			Headers:   map[string]string{"message_type": strconv.Itoa(messageType)},
			Timestamp: time.Now().Unix(),
		}

		if err := tp.sendMessage(tunnelMsg); err != nil {
			log.Printf("Failed to forward WebSocket message: %v", err)
			break
		}
	}
}

func (tp *TunnelProtocol) sendMessage(message *TunnelMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return tp.conn.WriteMessage(websocket.TextMessage, data)
}

// SendMessage is a public method to send messages
func (tp *TunnelProtocol) SendMessage(message *TunnelMessage) error {
	return tp.sendMessage(message)
}

// SendPing sends a ping message to the agent
func (tp *TunnelProtocol) SendPing() error {
	pingMessage := &TunnelMessage{
		Type:      "ping",
		ID:        fmt.Sprintf("%s-ping-%d", tp.tunnelID, time.Now().Unix()),
		Timestamp: time.Now().Unix(),
	}
	return tp.sendMessage(pingMessage)
}

// SendTerminate sends a terminate message to the agent
func (tp *TunnelProtocol) SendTerminate() error {
	terminateMessage := &TunnelMessage{
		Type:      "terminate",
		ID:        fmt.Sprintf("%s-terminate-%d", tp.tunnelID, time.Now().Unix()),
		Timestamp: time.Now().Unix(),
	}
	return tp.sendMessage(terminateMessage)
}

// IsHealthy checks if the tunnel connection is healthy
func (tp *TunnelProtocol) IsHealthy() bool {
	// Implementation would track last pong received
	return tp.conn != nil
}

// Close closes the tunnel protocol connection
func (tp *TunnelProtocol) Close() error {
	// Close all pending request channels
	for id, ch := range tp.pendingReqs {
		close(ch)
		delete(tp.pendingReqs, id)
	}

	if tp.conn != nil {
		return tp.conn.Close()
	}
	return nil
}
