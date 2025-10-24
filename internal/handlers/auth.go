package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"skyport-server/internal/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	db        *sql.DB
	jwtSecret string
}

func NewAuthHandler(db *sql.DB, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		db:        db,
		jwtSecret: jwtSecret,
	}
}

func (h *AuthHandler) SignUp(c *gin.Context) {
	var req models.SignUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if user already exists
	var userExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", req.Email).Scan(&userExists)
	if err != nil {
		log.Printf("Failed to check user existence for email %s: %v", req.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if userExists {
		c.JSON(http.StatusConflict, gin.H{"error": "User already exists with this email"})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash password for email %s: %v", req.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	// Create user
	userID := uuid.New()
	_, err = h.db.Exec(
		"INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)",
		userID, req.Email, string(hashedPassword), req.Name,
	)
	if err != nil {
		log.Printf("Failed to create user with email %s: %v", req.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Generate tokens
	token, refreshToken, err := h.generateTokens(userID.String())
	if err != nil {
		log.Printf("Failed to generate tokens for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Save refresh token
	err = h.saveRefreshToken(userID, refreshToken)
	if err != nil {
		log.Printf("Failed to save refresh token for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save refresh token"})
		return
	}

	// Return user and tokens
	user := models.User{
		ID:    userID,
		Email: req.Email,
		Name:  req.Name,
	}

	c.JSON(http.StatusCreated, models.AuthResponse{
		Token:        token,
		RefreshToken: refreshToken,
		User:         user,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user from database
	var user models.User
	var passwordHash string
	err := h.db.QueryRow(
		"SELECT id, email, password_hash, name, created_at, updated_at FROM users WHERE email = $1",
		req.Email,
	).Scan(&user.ID, &user.Email, &passwordHash, &user.Name, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}
	if err != nil {
		log.Printf("Failed to fetch user for login with email %s: %v", req.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Check password
	err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Generate tokens
	token, refreshToken, err := h.generateTokens(user.ID.String())
	if err != nil {
		log.Printf("Failed to generate tokens for user %s: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Save refresh token
	err = h.saveRefreshToken(user.ID, refreshToken)
	if err != nil {
		log.Printf("Failed to save refresh token for user %s: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save refresh token"})
		return
	}

	c.JSON(http.StatusOK, models.AuthResponse{
		Token:        token,
		RefreshToken: refreshToken,
		User:         user,
	})
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate refresh token
	var userID uuid.UUID
	var expiresAt time.Time
	err := h.db.QueryRow(
		"SELECT user_id, expires_at FROM refresh_tokens WHERE token = $1",
		req.RefreshToken,
	).Scan(&userID, &expiresAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}
	if err != nil {
		log.Printf("Failed to validate refresh token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if time.Now().After(expiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token expired"})
		return
	}

	// Generate new tokens
	token, newRefreshToken, err := h.generateTokens(userID.String())
	if err != nil {
		log.Printf("Failed to generate new tokens for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Delete old refresh token and save new one
	_, err = h.db.Exec("DELETE FROM refresh_tokens WHERE token = $1", req.RefreshToken)
	if err != nil {
		log.Printf("Failed to delete old refresh token for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete old refresh token"})
		return
	}

	err = h.saveRefreshToken(userID, newRefreshToken)
	if err != nil {
		log.Printf("Failed to save new refresh token for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":         token,
		"refresh_token": newRefreshToken,
	})
}

func (h *AuthHandler) AgentAuth(c *gin.Context) {
	var req models.AgentAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Parse and validate the incoming browser token
	token, err := jwt.Parse(req.Token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(h.jwtSecret), nil
	})

	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
		return
	}

	userIDStr, ok := claims["user_id"].(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in token"})
		return
	}

	// Get user info
	var user models.User
	err = h.db.QueryRow(
		"SELECT id, email, name, created_at, updated_at FROM users WHERE id = $1",
		userIDStr,
	).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		log.Printf("Failed to fetch user %s for agent auth: %v", userIDStr, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Generate permanent agent service token (no expiry like Cloudflare/Ngrok)
	agentToken, err := h.generateAgentToken(userIDStr)
	if err != nil {
		log.Printf("Failed to generate agent token for user %s: %v", userIDStr, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate agent token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":       true,
		"user":        user,
		"agent_token": agentToken, // Permanent service token for agent
	})
}

func (h *AuthHandler) GetProfile(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Get user info
	var user models.User
	err := h.db.QueryRow(
		"SELECT id, email, name, created_at, updated_at FROM users WHERE id = $1",
		userIDStr,
	).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		log.Printf("Failed to fetch profile for user %s: %v", userIDStr, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// generateTokens creates browser tokens with industry-standard expiry times
func (h *AuthHandler) generateTokens(userID string) (string, string, error) {
	// Generate access token (expires in 1 hour - industry standard)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
		"type":    "access",
	})

	tokenString, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		return "", "", err
	}

	// Generate refresh token (expires in 30 days - industry standard like ChatGPT, Google)
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour * 24 * 30).Unix(), // 30 days
		"iat":     time.Now().Unix(),
		"type":    "refresh",
	})

	refreshTokenString, err := refreshToken.SignedString([]byte(h.jwtSecret))
	if err != nil {
		return "", "", err
	}

	return tokenString, refreshTokenString, nil
}

// generateAgentToken creates a permanent service token for agents (no expiry like Cloudflare/Ngrok)
func (h *AuthHandler) generateAgentToken(userID string) (string, error) {
	// Generate permanent agent token (no expiry - service token like Cloudflare Tunnel)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"iat":     time.Now().Unix(),
		"type":    "agent",
		"service": true, // Mark as service token
		// No "exp" claim = no expiry
	})

	tokenString, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func (h *AuthHandler) saveRefreshToken(userID uuid.UUID, refreshToken string) error {
	_, err := h.db.Exec(
		"INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)",
		userID, refreshToken, time.Now().Add(time.Hour*24*30), // 30 days
	)
	return err
}
