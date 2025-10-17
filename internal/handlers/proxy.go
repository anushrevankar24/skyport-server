package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type ProxyHandler struct {
	db            *sql.DB
	tunnelHandler *TunnelHandler
}

func NewProxyHandler(db *sql.DB, tunnelHandler *TunnelHandler) *ProxyHandler {
	return &ProxyHandler{
		db:            db,
		tunnelHandler: tunnelHandler,
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
		c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(`
<!DOCTYPE html>
<html>
<head>
    <title>Tunnel Not Found</title>
    <style>
        body { font-family: Arial, sans-serif; text-align: center; padding: 50px; }
        .container { max-width: 600px; margin: 0 auto; }
        .error { color: #e74c3c; }
        .info { color: #3498db; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <h1 class="error">Tunnel Not Found</h1>
        <p>The tunnel "<strong>`+subdomain+`</strong>" is not active or does not exist.</p>
        <div class="info">
            <p>To create this tunnel:</p>
            <ol>
                <li>Go to <a href="/">SkyPort Dashboard</a></li>
                <li>Create a new tunnel with subdomain "<strong>`+subdomain+`</strong>"</li>
                <li>Connect the tunnel using SkyPort Agent</li>
            </ol>
        </div>
    </div>
</body>
</html>
		`))
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if !isActive {
		c.Data(http.StatusServiceUnavailable, "text/html; charset=utf-8", []byte(`
<!DOCTYPE html>
<html>
<head>
    <title>Tunnel Offline</title>
    <style>
        body { font-family: Arial, sans-serif; text-align: center; padding: 50px; }
        .container { max-width: 600px; margin: 0 auto; }
        .warning { color: #f39c12; }
        .info { color: #3498db; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <h1 class="warning">Tunnel Offline</h1>
        <p>The tunnel "<strong>`+subdomain+`</strong>" exists but is not currently connected.</p>
        <div class="info">
            <p>To activate this tunnel:</p>
            <ol>
                <li>Open SkyPort Agent on your computer</li>
                <li>Sign in to your account</li>
                <li>Click "Connect" next to the "<strong>`+subdomain+`</strong>" tunnel</li>
            </ol>
        </div>
    </div>
</body>
</html>
		`))
		return
	}

	// Check if we have an active tunnel connection
	tunnel, exists := h.tunnelHandler.GetActiveTunnel(tunnelID)
	if !exists {
		c.Data(http.StatusServiceUnavailable, "text/html; charset=utf-8", []byte(`
<!DOCTYPE html>
<html>
<head>
    <title>Tunnel Connection Lost</title>
    <style>
        body { font-family: Arial, sans-serif; text-align: center; padding: 50px; }
        .container { max-width: 600px; margin: 0 auto; }
        .warning { color: #f39c12; }
        .info { color: #3498db; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <h1 class="warning">Tunnel Connection Lost</h1>
        <p>The tunnel "<strong>`+subdomain+`</strong>" is marked as active but no connection exists.</p>
        <div class="info">
            <p>Please reconnect your tunnel from SkyPort Agent.</p>
        </div>
    </div>
</body>
</html>
		`))
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
