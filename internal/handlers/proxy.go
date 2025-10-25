package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"skyport-server/internal/config"
	"skyport-server/internal/templates"
	"strings"

	"github.com/gin-gonic/gin"
)

type ProxyHandler struct {
	db            *sql.DB
	tunnelHandler *TunnelHandler
	config        *config.Config
}

func NewProxyHandler(db *sql.DB, tunnelHandler *TunnelHandler, cfg *config.Config) *ProxyHandler {
	return &ProxyHandler{
		db:            db,
		tunnelHandler: tunnelHandler,
		config:        cfg,
	}
}

// HandleSubdomain handles requests to subdomains and proxies them to local tunnels
func (h *ProxyHandler) HandleSubdomain(c *gin.Context) {
	host := c.Request.Host

	// Extract subdomain from host
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invalid subdomain"})
		return
	}

	subdomain := parts[0]

	// Skip localhost itself
	if subdomain == "localhost" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No tunnel found"})
		return
	}

	// Find active tunnel for this subdomain
	var tunnelID, userID string
	var localPort int
	var isActive bool

	err := h.db.QueryRow(`
		SELECT id, user_id, local_port, is_active 
		FROM tunnels 
		WHERE subdomain = $1 AND is_active = true
	`, subdomain).Scan(&tunnelID, &userID, &localPort, &isActive)

	if err == sql.ErrNoRows {
		dashboardURL := h.config.WebAppURL + "/dashboard"
		html, err := templates.RenderTunnelNotFound(subdomain, dashboardURL)
		if err != nil {
			log.Printf("Failed to render template: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Template error"})
			return
		}
		c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(html))
		return
	}

	if err != nil {
		log.Printf("Failed to query tunnel for subdomain %s: %v", subdomain, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if !isActive {
		dashboardURL := h.config.WebAppURL + "/dashboard"
		html, err := templates.RenderTunnelOffline(subdomain, dashboardURL)
		if err != nil {
			log.Printf("Failed to render template: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Template error"})
			return
		}
		c.Data(http.StatusServiceUnavailable, "text/html; charset=utf-8", []byte(html))
		return
	}

	// Check if we have an active tunnel connection
	tunnel, exists := h.tunnelHandler.GetActiveTunnel(tunnelID)
	if !exists {
		dashboardURL := h.config.WebAppURL + "/dashboard"
		html, err := templates.RenderTunnelConnectionLost(subdomain, dashboardURL)
		if err != nil {
			log.Printf("Failed to render template: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Template error"})
			return
		}
		c.Data(http.StatusServiceUnavailable, "text/html; charset=utf-8", []byte(html))
		return
	}

	// Check if this is a WebSocket upgrade request
	if isWebSocketUpgrade(c.Request) {
		tunnel.HandleWebSocketUpgrade(c.Writer, c.Request)
	} else {
		// Handle regular HTTP request through tunnel
		tunnel.HandleIncomingHTTPRequest(c.Writer, c.Request)
	}
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade request
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Connection")) == "upgrade" &&
		strings.ToLower(r.Header.Get("Upgrade")) == "websocket"
}
