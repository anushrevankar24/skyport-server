package handlers

import (
	"database/sql"
	"log"
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan tunnel"})
			return
		}
		tunnels = append(tunnels, tunnel)
	}

	if tunnels == nil {
		tunnels = []models.Tunnel{}
	}

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete tunnel"})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
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

	// Update tunnel as active
	_, err = h.db.Exec(
		"UPDATE tunnels SET is_active = true, last_seen = NOW(), connected_ip = $1 WHERE id = $2",
		c.ClientIP(), tunnelID,
	)
	if err != nil {
		log.Printf("Failed to update tunnel status: %v", err)
		return
	}

	log.Printf("Tunnel %s connected from user %s", tunnelID, userIDStr)

	// Get tunnel info for local port
	var localPort int
	err = h.db.QueryRow("SELECT local_port FROM tunnels WHERE id = $1", tunnelID).Scan(&localPort)
	if err != nil {
		log.Printf("Failed to get tunnel local port: %v", err)
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

	// Handle messages from agent in a goroutine
	go func() {
		for {
			_, message, err := tunnelConn.Conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				return
			}

			// Handle tunnel protocol messages
			if err := protocol.HandleTunnelMessage(message); err != nil {
				log.Printf("Failed to handle tunnel message: %v", err)
			}

			// Update last seen timestamp on any message from agent
			_, err = h.db.Exec("UPDATE tunnels SET last_seen = NOW() WHERE id = $1", tunnelConn.TunnelID)
			if err != nil {
				log.Printf("Failed to update last seen: %v", err)
			}

			// Refresh heartbeat on any received message
			lastHeartbeat = time.Now()
		}
	}()

	// Heartbeat monitoring loop
	heartbeatTicker := time.NewTicker(15 * time.Second) // Check every 15 seconds
	defer heartbeatTicker.Stop()

	for range heartbeatTicker.C {
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

		// Send ping to agent
		if err := protocol.SendPing(); err != nil {
			log.Printf("Failed to send ping to tunnel %s: %v", tunnelConn.TunnelID, err)
			return
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
