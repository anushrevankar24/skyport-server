package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Email     string    `json:"email" db:"email"`
	Password  string    `json:"-" db:"password_hash"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type Tunnel struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	UserID      uuid.UUID  `json:"user_id" db:"user_id"`
	Name        string     `json:"name" db:"name"`
	Subdomain   string     `json:"subdomain" db:"subdomain"`
	LocalPort   int        `json:"local_port" db:"local_port"`
	AuthToken   string     `json:"auth_token" db:"auth_token"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	LastSeen    *time.Time `json:"last_seen" db:"last_seen"`
	ConnectedIP *string    `json:"connected_ip" db:"connected_ip"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

type AuthResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	User         User   `json:"user"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type SignUpRequest struct {
	Name     string `json:"name" binding:"required,min=2"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type CreateTunnelRequest struct {
	Name      string `json:"name" binding:"required,min=1"`
	Subdomain string `json:"subdomain" binding:"required,min=3,max=20"`
	LocalPort int    `json:"local_port" binding:"required,min=1,max=65535"`
}

type AgentAuthRequest struct {
	Token string `json:"token" binding:"required"`
}




