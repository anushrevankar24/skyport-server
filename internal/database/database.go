package database

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Initialize(databaseURL string) (*sql.DB, error) {
	// Use pgx driver which works better with Supabase connection pooler
	// Disable prepared statement caching to avoid conflicts with connection poolers
	// by adding statement_cache_mode=describe to the connection string
	if databaseURL != "" {
		// Check if URL already has query parameters
		separator := "?"
		if containsQueryParams(databaseURL) {
			separator = "&"
		}
		// Add statement_cache_mode=describe to disable prepared statement caching
		databaseURL += separator + "statement_cache_mode=describe"
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings optimized for connection pooler
	// Lower values work better with poolers like PgBouncer/Supabase pooler
	db.SetMaxOpenConns(10)   // Reduced for pooler efficiency
	db.SetMaxIdleConns(2)    // Keep minimal idle connections
	db.SetConnMaxLifetime(0) // Reuse connections indefinitely
	db.SetConnMaxIdleTime(0) // Don't close idle connections

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// containsQueryParams checks if a database URL already has query parameters
func containsQueryParams(url string) bool {
	for _, char := range url {
		if char == '?' {
			return true
		}
	}
	return false
}

func RunMigrations(db *sql.DB) error {
	migrations := []string{
		`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`,

		`CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			name VARCHAR(255) NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);`,

		`CREATE TABLE IF NOT EXISTS tunnels (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(255) NOT NULL,
			subdomain VARCHAR(255) UNIQUE NOT NULL,
			local_port INTEGER NOT NULL,
			auth_token VARCHAR(255) UNIQUE NOT NULL,
			is_active BOOLEAN DEFAULT FALSE,
			last_seen TIMESTAMP WITH TIME ZONE,
			connected_ip VARCHAR(45),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);`,

		`CREATE INDEX IF NOT EXISTS idx_tunnels_user_id ON tunnels(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_tunnels_subdomain ON tunnels(subdomain);`,
		`CREATE INDEX IF NOT EXISTS idx_tunnels_auth_token ON tunnels(auth_token);`,

		`CREATE TABLE IF NOT EXISTS refresh_tokens (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token VARCHAR(255) UNIQUE NOT NULL,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);`,

		`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_token ON refresh_tokens(token);`,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("failed to run migration: %w", err)
		}
	}

	return nil
}
