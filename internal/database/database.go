package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Initialize(databaseURL string) (*sql.DB, error) {
	// Detect pooler type and configure accordingly
	// Transaction poolers (port 6543) don't support prepared statements at all
	// Session poolers (port 5432) can use statement caching
	isTransactionPooler := strings.Contains(databaseURL, ":6543") ||
		strings.Contains(databaseURL, "port=6543")

	if databaseURL != "" {
		// Check if URL already has query parameters or uses DSN format
		separator := "?"
		if containsQueryParams(databaseURL) {
			separator = "&"
		}
		// DSN format uses space-separated key=value pairs
		if strings.Contains(databaseURL, "host=") && !strings.Contains(databaseURL, "://") {
			separator = " "
		}

		if isTransactionPooler {
			// Transaction poolers (port 6543) are NOT RECOMMENDED for this application
			// They don't support prepared statements and cause connection issues
			log.Println("⚠️  WARNING: Transaction pooler detected (port 6543)")
			log.Println("⚠️  Transaction poolers are NOT recommended for applications with WebSockets")
			log.Println("⚠️  Please switch to Session Pooler (port 5432) for better stability")
			log.Println("⚠️  Attempting to disable prepared statements, but issues may still occur...")

			// Must use prefer_simple_protocol=yes to disable them completely
			databaseURL += separator + "prefer_simple_protocol=yes"
			// Also explicitly disable statement cache
			if separator == " " {
				databaseURL += " statement_cache_capacity=0"
			} else {
				databaseURL += "&statement_cache_capacity=0"
			}
		} else if needsStatementCacheMode(databaseURL) {
			// Session poolers (port 5432) can use statement caching safely
			databaseURL += separator + "statement_cache_mode=describe"
			log.Println("✓ Session pooler detected - configured with statement_cache_mode=describe")
		}
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings optimized for session pooler
	// These settings work well with Supabase session pooler (port 5432)
	db.SetMaxOpenConns(20)   // Reasonable for session pooler
	db.SetMaxIdleConns(5)    // Keep some idle connections ready
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

// needsStatementCacheMode detects if we're connecting through a connection pooler
// Connection poolers like Supabase and PgBouncer support and need statement_cache_mode
// Local PostgreSQL installations do not support this parameter
func needsStatementCacheMode(url string) bool {
	// First, check for local PostgreSQL - these DON'T need statement_cache_mode
	localIndicators := []string{"localhost", "127.0.0.1", "host=localhost", "host=127.0.0.1"}
	for _, indicator := range localIndicators {
		if contains(url, indicator) {
			log.Println("✓ Local PostgreSQL detected - using standard configuration")
			return false
		}
	}

	// Connection pooler indicators in the URL
	poolerIndicators := []string{
		"supabase.co",     // Supabase hosted
		"pooler.supabase", // Supabase pooler
		"pgbouncer",       // PgBouncer
		":5432",           // Session pooler on standard port (when not local)
		"port=5432",       // Session pooler DSN format
		"pooler=true",     // Explicit pooler flag
	}

	// Check if any pooler indicator is present
	for _, indicator := range poolerIndicators {
		if contains(url, indicator) {
			return true
		}
	}

	return false
}

// contains checks if a string contains a substring
func contains(str, substr string) bool {
	return len(str) >= len(substr) &&
		(str == substr ||
			str[:len(substr)] == substr ||
			str[len(str)-len(substr):] == substr ||
			containsMiddle(str, substr))
}

func containsMiddle(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
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
