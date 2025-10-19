package config

import (
	"regexp"
	"strings"
)

// ReservedSubdomains contains all subdomains reserved for system use
var ReservedSubdomains = []string{
	// Core infrastructure
	"web", "app", "www", "api", "admin", "dashboard", "console",
	"portal", "control", "panel", "cp", "manage", "manager",

	// Authentication & Security
	"auth", "login", "signup", "register", "account", "accounts",
	"oauth", "sso", "identity", "id", "session", "sessions",
	"security", "secure", "verify", "verification",

	// Email services
	"mail", "email", "smtp", "pop", "pop3", "imap",
	"webmail", "mta", "mx", "postmaster", "abuse",

	// Network services
	"ftp", "sftp", "ssh", "vpn", "proxy", "gateway",
	"tunnel", "tunnels", "agent", "agents", "client", "clients",
	"dns", "ns", "ns1", "ns2", "ns3", "ns4",

	// Development & Testing
	"dev", "develop", "development", "staging", "stage",
	"test", "testing", "qa", "uat", "demo", "sandbox",
	"preview", "beta", "alpha", "canary", "edge",

	// Production & Operations
	"prod", "production", "live", "internal", "private",
	"ops", "devops", "sre", "infrastructure", "infra",

	// Documentation & Support
	"docs", "documentation", "wiki", "help", "support",
	"helpdesk", "faq", "guide", "guides", "tutorial", "tutorials",
	"kb", "knowledgebase", "learn", "learning",

	// Community & Social
	"blog", "news", "forum", "forums", "community",
	"social", "chat", "discuss", "discussion", "discussions",

	// Commerce & Payments
	"store", "shop", "cart", "checkout", "payment", "payments",
	"billing", "invoice", "invoices", "pay", "purchase",
	"order", "orders", "product", "products",

	// Content Delivery
	"cdn", "static", "assets", "media", "images", "img",
	"files", "file", "download", "downloads", "upload", "uploads",
	"content", "data", "storage", "s3", "bucket",

	// AI & Analytics
	"ai", "ml", "machinelearning", "artificialintelligence",
	"bot", "bots", "chatbot", "analytics", "metrics",
	"stats", "statistics", "monitoring", "monitor",
	"status", "health", "check", "ping",

	// API & Webhooks
	"api1", "api2", "apiv1", "apiv2", "rest", "graphql",
	"webhook", "webhooks", "callback", "callbacks",
	"integration", "integrations", "connect", "sync",

	// Database & Backend
	"db", "database", "mysql", "postgres", "postgresql",
	"mongodb", "redis", "cache", "queue", "worker", "workers",
	"job", "jobs", "task", "tasks", "cron",

	// Mobile & Apps
	"mobile", "m", "ios", "android", "app-store", "play",
	"download-app", "get-app", "app-download",

	// Legal & Corporate
	"legal", "terms", "tos", "privacy", "policy", "policies",
	"gdpr", "compliance", "copyright", "dmca",
	"about", "contact", "careers", "jobs",

	// Marketing & Sales
	"marketing", "promo", "promotion", "promotions",
	"campaign", "campaigns", "landing", "lp",
	"sales", "crm", "lead", "leads",

	// Monitoring & Logging
	"logs", "logging", "trace", "tracing", "audit",
	"sentry", "bugsnag", "errors", "error",
	"uptime", "downtime", "incident", "incidents",

	// Testing & Automation
	"ci", "cd", "jenkins", "travis", "circleci",
	"gitlab", "github", "bitbucket", "git",
	"build", "builds", "deploy", "deployment", "deployments",

	// Reserved for common uses
	"localhost", "local", "root", "system", "sys",
	"server", "servers", "host", "hosts", "node", "nodes",
	"service", "services", "microservice", "microservices",

	// Prevent abuse
	"admin1", "admin2", "administrator", "superuser",
	"root-admin", "sysadmin", "hostmaster", "webmaster",
	"postfix", "dovecot", "apache", "nginx",

	// Cloud & Infrastructure
	"cloud", "aws", "azure", "gcp", "digitalocean",
	"heroku", "vercel", "netlify", "cloudflare",
	"kubernetes", "k8s", "docker", "container", "containers",

	// User-facing features (future)
	"profile", "profiles", "user", "users", "member", "members",
	"team", "teams", "organization", "organizations", "org", "orgs",
	"workspace", "workspaces", "project", "projects",

	// Reserved TLDs and common patterns
	"email-verify", "reset-password", "forgot-password",
	"change-password", "update-email", "confirm-email",
	"activate", "activation", "deactivate", "suspend", "suspended",
}

// IsReservedSubdomain checks if a subdomain is in the reserved list
func IsReservedSubdomain(subdomain string) bool {
	subdomainLower := strings.ToLower(subdomain)
	for _, reserved := range ReservedSubdomains {
		if subdomainLower == reserved {
			return true
		}
	}
	return false
}

// ValidateSubdomain performs comprehensive validation on a subdomain
func ValidateSubdomain(subdomain string) (bool, string) {
	subdomainLower := strings.ToLower(subdomain)

	// Check length (3-63 characters per DNS standards)
	if len(subdomainLower) < 3 {
		return false, "Subdomain must be at least 3 characters long"
	}
	if len(subdomainLower) > 63 {
		return false, "Subdomain cannot exceed 63 characters"
	}

	// Check if reserved
	if IsReservedSubdomain(subdomainLower) {
		return false, "This subdomain is reserved for system use. Please choose a different name."
	}

	// Validate format: alphanumeric and hyphens only, cannot start or end with hyphen
	validSubdomain := regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	if !validSubdomain.MatchString(subdomainLower) {
		return false, "Subdomain must contain only lowercase letters, numbers, and hyphens. It cannot start or end with a hyphen."
	}

	// Prevent consecutive hyphens (optional, but good practice)
	if strings.Contains(subdomainLower, "--") {
		return false, "Subdomain cannot contain consecutive hyphens"
	}

	return true, ""
}
