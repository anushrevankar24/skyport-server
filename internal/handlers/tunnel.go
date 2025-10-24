package handlers

import (
	"database/sql"
	"log"
	"net"
	"net/http"
	"skyport-server/internal/config"
	"skyport-server/internal/models"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type TunnelHandler struct {
	db            *sql.DB
	upgrader      websocket.Upgrader
	activeTunnels map[string]*TunnelProtocol
	tunnelsMutex  sync.RWMutex
}

type TunnelConnection struct {
	TunnelID string
	UserID   string
	Conn     *websocket.Conn
}

func NewTunnelHandler(db *sql.DB) *TunnelHandler {
	return &TunnelHandler{
		db:            db,
		activeTunnels: make(map[string]*TunnelProtocol),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
			// Enable compression for better performance over slow connections
			EnableCompression: true,
		},
	}
}

func (h *TunnelHandler) GetTunnels(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	rows, err := h.db.Query(`
		SELECT id, user_id, name, subdomain, local_port, auth_token, is_active, last_seen, connected_ip, created_at, updated_at 
		FROM tunnels 
		WHERE user_id = $1 
		ORDER BY created_at DESC
	`, userIDStr)
	if err != nil {
		log.Printf("Failed to fetch tunnels for user %s: %v", userIDStr, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tunnels"})
		return
	}
	defer rows.Close()

	var tunnels []models.Tunnel
	for rows.Next() {
		var tunnel models.Tunnel
		err := rows.Scan(
			&tunnel.ID, &tunnel.UserID, &tunnel.Name, &tunnel.Subdomain,
			&tunnel.LocalPort, &tunnel.AuthToken, &tunnel.IsActive,
			&tunnel.LastSeen, &tunnel.ConnectedIP, &tunnel.CreatedAt, &tunnel.UpdatedAt,
		)
		if err != nil {
			log.Printf("Failed to scan tunnel for user %s: %v", userIDStr, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan tunnel"})
			return
		}
		tunnels = append(tunnels, tunnel)
	}

	if tunnels == nil {
		tunnels = []models.Tunnel{}
	}

	// Enhance with real-time data from memory for active tunnels
	h.tunnelsMutex.RLock()
	for i := range tunnels {
		if protocol, exists := h.activeTunnels[tunnels[i].ID.String()]; exists {
			// Get real-time status from memory
			tunnels[i].LastSeen = &protocol.lastHeartbeat
			// Consider active if heartbeat is less than 45 seconds old
			tunnels[i].IsActive = time.Since(protocol.lastHeartbeat) < 45*time.Second
		}
	}
	h.tunnelsMutex.RUnlock()

	c.JSON(http.StatusOK, gin.H{"tunnels": tunnels})
}

func (h *TunnelHandler) CreateTunnel(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req models.CreateTunnelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate subdomain
	isValid, validationError := config.ValidateSubdomain(req.Subdomain)
	if !isValid {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationError})
		return
	}

	// Check if subdomain already exists
	var subdomainExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM tunnels WHERE subdomain = $1)", req.Subdomain).Scan(&subdomainExists)
	if err != nil {
		log.Printf("Failed to check subdomain existence for %s: %v", req.Subdomain, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if subdomainExists {
		c.JSON(http.StatusConflict, gin.H{"error": "Subdomain already exists"})
		return
	}

	// Generate auth token for tunnel
	authToken := uuid.New().String()
	tunnelID := uuid.New()

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Create tunnel
	_, err = h.db.Exec(`
		INSERT INTO tunnels (id, user_id, name, subdomain, local_port, auth_token) 
		VALUES ($1, $2, $3, $4, $5, $6)
	`, tunnelID, userID, req.Name, req.Subdomain, req.LocalPort, authToken)
	if err != nil {
		log.Printf("Failed to create tunnel %s for user %s: %v", req.Name, userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create tunnel"})
		return
	}

	// Return created tunnel
	tunnel := models.Tunnel{
		ID:        tunnelID,
		UserID:    userID,
		Name:      req.Name,
		Subdomain: req.Subdomain,
		LocalPort: req.LocalPort,
		AuthToken: authToken,
		IsActive:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	c.JSON(http.StatusCreated, tunnel)
}

func (h *TunnelHandler) DeleteTunnel(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	tunnelID := c.Param("id")

	// Delete tunnel (only if it belongs to the user)
	result, err := h.db.Exec("DELETE FROM tunnels WHERE id = $1 AND user_id = $2", tunnelID, userIDStr)
	if err != nil {
		log.Printf("Failed to delete tunnel %s for user %s: %v", tunnelID, userIDStr, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete tunnel"})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Failed to check deletion result for tunnel %s: %v", tunnelID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check deletion result"})
		return
	}

	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tunnel not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tunnel deleted successfully"})
}

func (h *TunnelHandler) ConnectTunnel(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Get tunnel ID and auth token from headers
	tunnelID := c.GetHeader("X-Tunnel-ID")
	tunnelAuth := c.GetHeader("X-Tunnel-Auth")

	if tunnelID == "" || tunnelAuth == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing tunnel credentials"})
		return
	}

	// Validate tunnel ownership and auth token
	var dbTunnelAuth string
	var dbUserID string
	err := h.db.QueryRow(
		"SELECT auth_token, user_id FROM tunnels WHERE id = $1",
		tunnelID,
	).Scan(&dbTunnelAuth, &dbUserID)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tunnel not found"})
		return
	}
	if err != nil {
		log.Printf("Failed to fetch tunnel %s from database: %v", tunnelID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Verify user owns this tunnel
	if dbUserID != userIDStr {
		c.JSON(http.StatusForbidden, gin.H{"error": "Tunnel does not belong to user"})
		return
	}

	// Verify auth token
	if dbTunnelAuth != tunnelAuth {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid tunnel auth token"})
		return
	}

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	// Enable TCP keepalive on the underlying connection
	// This is critical for maintaining long-lived connections through NAT/firewalls
	if tcpConn, ok := conn.UnderlyingConn().(*net.TCPConn); ok {
		if err := tcpConn.SetKeepAlive(true); err != nil {
			log.Printf("Failed to enable TCP keepalive for tunnel %s: %v", tunnelID, err)
		} else {
			// Send keepalive probes every 30 seconds
			// This keeps NAT/firewall entries alive and detects dead connections
			if err := tcpConn.SetKeepAlivePeriod(30 * time.Second); err != nil {
				log.Printf("Failed to set TCP keepalive period for tunnel %s: %v", tunnelID, err)
			} else {
				log.Printf("TCP keepalive enabled for tunnel %s (30s interval)", tunnelID)
			}
		}

		// Optional: Set TCP buffer sizes for better performance
		tcpConn.SetReadBuffer(64 * 1024)
		tcpConn.SetWriteBuffer(64 * 1024)
	}

	// Update tunnel as active
	_, err = h.db.Exec(
		"UPDATE tunnels SET is_active = true, last_seen = NOW(), connected_ip = $1 WHERE id = $2",
		c.ClientIP(), tunnelID,
	)
	if err != nil {
		log.Printf("ERROR: Failed to update tunnel status for %s: %v", tunnelID, err)
		// Send error message to agent before closing
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"terminate","id":"`+tunnelID+`","error":"Database error"}`))
		return
	}

	log.Printf("Tunnel %s connected from user %s", tunnelID, userIDStr)

	// Get tunnel info for local port
	var localPort int
	err = h.db.QueryRow("SELECT local_port FROM tunnels WHERE id = $1", tunnelID).Scan(&localPort)
	if err != nil {
		log.Printf("ERROR: Failed to get tunnel local port for %s: %v", tunnelID, err)
		// Send error message to agent before closing
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"terminate","id":"`+tunnelID+`","error":"Database error"}`))
		return
	}

	// Create tunnel protocol handler
	tunnelProtocol := NewTunnelProtocol(conn, tunnelID, localPort)

	// Store active tunnel
	h.tunnelsMutex.Lock()
	h.activeTunnels[tunnelID] = tunnelProtocol
	h.tunnelsMutex.Unlock()

	// Handle tunnel connection
	h.handleTunnelConnection(&TunnelConnection{
		TunnelID: tunnelID,
		UserID:   userIDStr.(string),
		Conn:     conn,
	}, tunnelProtocol)

	// Remove from active tunnels
	h.tunnelsMutex.Lock()
	delete(h.activeTunnels, tunnelID)
	h.tunnelsMutex.Unlock()

	// Update tunnel as inactive when connection ends
	_, err = h.db.Exec(
		"UPDATE tunnels SET is_active = false, last_seen = NOW() WHERE id = $1",
		tunnelID,
	)
	if err != nil {
		log.Printf("Failed to update tunnel status on disconnect: %v", err)
	}

	log.Printf("Tunnel %s disconnected", tunnelID)
}

func (h *TunnelHandler) handleTunnelConnection(tunnelConn *TunnelConnection, protocol *TunnelProtocol) {
	// Send connection confirmation
	connectedMsg := &TunnelMessage{
		Type:      "connected",
		ID:        tunnelConn.TunnelID,
		Timestamp: time.Now().Unix(),
	}
	if err := protocol.SendMessage(connectedMsg); err != nil {
		log.Printf("Failed to send connection confirmation: %v", err)
		return
	}

	// Track last heartbeat time
	lastHeartbeat := time.Now()
	heartbeatTimeout := 45 * time.Second // Mark inactive if no heartbeat for 45 seconds

	// Set up ping handler to respond to agent's WebSocket control frame pings
	tunnelConn.Conn.SetPingHandler(func(appData string) error {
		// Extend read deadline when we receive a ping
		tunnelConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		// Send pong response with write deadline
		err := tunnelConn.Conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
		if err != nil {
			log.Printf("Failed to send pong to tunnel %s: %v", tunnelConn.TunnelID, err)
		}
		lastHeartbeat = time.Now()
		protocol.lastHeartbeat = time.Now()
		return err
	})

	// Set up pong handler to detect when agent responds to our pings
	tunnelConn.Conn.SetPongHandler(func(appData string) error {
		// Extend read deadline when we receive a pong
		tunnelConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		lastHeartbeat = time.Now()
		protocol.lastHeartbeat = time.Now()
		return nil
	})

	// Set initial read deadline (60 seconds allows time for first ping/pong exchange)
	if err := tunnelConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		log.Printf("Failed to set initial read deadline for tunnel %s: %v", tunnelConn.TunnelID, err)
		return
	}

	// Channel to signal when read goroutine exits
	readDone := make(chan struct{})

	// Handle messages from agent in a goroutine
	go func() {
		defer close(readDone)
		for {
			_, message, err := tunnelConn.Conn.ReadMessage()
			if err != nil {
				// Log all connection errors for debugging
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("Tunnel %s closed gracefully: %v", tunnelConn.TunnelID, err)
				} else if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("Tunnel %s unexpected close: %v", tunnelConn.TunnelID, err)
				} else {
					log.Printf("Tunnel %s read error: %v", tunnelConn.TunnelID, err)
				}
				return
			}

			// Extend read deadline on successful read (application-level messages)
			tunnelConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			// Handle tunnel protocol messages
			if err := protocol.HandleTunnelMessage(message); err != nil {
				log.Printf("Failed to handle tunnel message: %v", err)
			}

			// Refresh heartbeat on any received message
			lastHeartbeat = time.Now()
			protocol.lastHeartbeat = time.Now()
		}
	}()

	// Heartbeat monitoring loop - send WebSocket control frame pings
	heartbeatTicker := time.NewTicker(15 * time.Second) // Send ping every 15 seconds
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-readDone:
			// Read goroutine exited, connection is closed
			log.Printf("Tunnel %s read goroutine exited", tunnelConn.TunnelID)
			return
		case <-heartbeatTicker.C:
			// Check if we've received a heartbeat recently
			if time.Since(lastHeartbeat) > heartbeatTimeout {
				log.Printf("Tunnel %s heartbeat timeout - marking as inactive", tunnelConn.TunnelID)
				// Mark tunnel as inactive due to heartbeat timeout
				_, err := h.db.Exec("UPDATE tunnels SET is_active = false WHERE id = $1", tunnelConn.TunnelID)
				if err != nil {
					log.Printf("Failed to mark tunnel as inactive: %v", err)
				}
				return
			}

			// Send WebSocket control frame ping to agent
			err := tunnelConn.Conn.WriteControl(
				websocket.PingMessage,
				[]byte{},
				time.Now().Add(10*time.Second),
			)
			if err != nil {
				log.Printf("Failed to send ping to tunnel %s: %v", tunnelConn.TunnelID, err)
				return
			}
		}
	}
}

// StopTunnel stops an active tunnel by sending a terminate message
func (h *TunnelHandler) StopTunnel(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	tunnelID := c.Param("id")
	if tunnelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tunnel ID is required"})
		return
	}

	// Verify user owns this tunnel
	var dbUserID string
	err := h.db.QueryRow("SELECT user_id FROM tunnels WHERE id = $1", tunnelID).Scan(&dbUserID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tunnel not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if dbUserID != userIDStr {
		c.JSON(http.StatusForbidden, gin.H{"error": "Tunnel does not belong to user"})
		return
	}

	// Check if tunnel is active and send terminate message
	h.tunnelsMutex.RLock()
	protocol, exists := h.activeTunnels[tunnelID]
	h.tunnelsMutex.RUnlock()

	if !exists {
		// No in-memory connection, but DB may still show active due to a stale state
		// Force-mark the tunnel as inactive to reconcile state and return 200
		if _, err := h.db.Exec("UPDATE tunnels SET is_active = false, last_seen = NOW() WHERE id = $1", tunnelID); err != nil {
			log.Printf("Failed to reconcile inactive tunnel %s: %v", tunnelID, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Tunnel is not currently active"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Tunnel was not connected; marked inactive"})
		return
	}

	// Send terminate message to agent
	if err := protocol.SendTerminate(); err != nil {
		log.Printf("Failed to send terminate message to tunnel %s: %v", tunnelID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to stop tunnel"})
		return
	}

	// Mark tunnel as inactive in database
	_, err = h.db.Exec("UPDATE tunnels SET is_active = false WHERE id = $1", tunnelID)
	if err != nil {
		log.Printf("Failed to update tunnel status: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tunnel stop signal sent successfully"})
}

// GetActiveTunnel returns the active tunnel protocol for a given tunnel ID
func (h *TunnelHandler) GetActiveTunnel(tunnelID string) (*TunnelProtocol, bool) {
	h.tunnelsMutex.RLock()
	defer h.tunnelsMutex.RUnlock()
	tunnel, exists := h.activeTunnels[tunnelID]
	return tunnel, exists
}
