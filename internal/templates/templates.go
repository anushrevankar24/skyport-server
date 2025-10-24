package templates

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"sync"
)

//go:embed *.html
var templatesFS embed.FS

var (
	templates *template.Template
	once      sync.Once
	loadErr   error
)

// Initialize loads all templates on first use
func Initialize() error {
	once.Do(func() {
		templates, loadErr = template.ParseFS(templatesFS, "*.html")
	})
	return loadErr
}

// ErrorPageData contains data for error pages
type ErrorPageData struct {
	Title        string
	ErrorCode    string
	Message      string
	Instructions template.HTML // Use template.HTML to allow HTML in instructions
}

// TunnelErrorData contains data for tunnel-specific errors
type TunnelErrorData struct {
	Subdomain string
}

// LocalServiceErrorData contains data for local service connection errors
type LocalServiceErrorData struct {
	Title     string
	ErrorCode string
	Message   string
	LocalPort int
	ErrorType string // "connection_refused", "timeout", "other"
}

// RenderErrorPage renders the error_base.html template
func RenderErrorPage(data ErrorPageData) (string, error) {
	if err := Initialize(); err != nil {
		return "", fmt.Errorf("failed to initialize templates: %w", err)
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "error_base.html", data); err != nil {
		return "", fmt.Errorf("failed to render error page: %w", err)
	}
	return buf.String(), nil
}

// RenderTunnelNotFound renders the tunnel_not_found.html template
func RenderTunnelNotFound(subdomain string) (string, error) {
	if err := Initialize(); err != nil {
		return "", fmt.Errorf("failed to initialize templates: %w", err)
	}

	var buf bytes.Buffer
	data := TunnelErrorData{Subdomain: subdomain}
	if err := templates.ExecuteTemplate(&buf, "tunnel_not_found.html", data); err != nil {
		return "", fmt.Errorf("failed to render tunnel not found page: %w", err)
	}
	return buf.String(), nil
}

// RenderTunnelOffline renders the tunnel_offline.html template
func RenderTunnelOffline(subdomain string) (string, error) {
	if err := Initialize(); err != nil {
		return "", fmt.Errorf("failed to initialize templates: %w", err)
	}

	var buf bytes.Buffer
	data := TunnelErrorData{Subdomain: subdomain}
	if err := templates.ExecuteTemplate(&buf, "tunnel_offline.html", data); err != nil {
		return "", fmt.Errorf("failed to render tunnel offline page: %w", err)
	}
	return buf.String(), nil
}

// RenderTunnelConnectionLost renders the tunnel_connection_lost.html template
func RenderTunnelConnectionLost(subdomain string) (string, error) {
	if err := Initialize(); err != nil {
		return "", fmt.Errorf("failed to initialize templates: %w", err)
	}

	var buf bytes.Buffer
	data := TunnelErrorData{Subdomain: subdomain}
	if err := templates.ExecuteTemplate(&buf, "tunnel_connection_lost.html", data); err != nil {
		return "", fmt.Errorf("failed to render tunnel connection lost page: %w", err)
	}
	return buf.String(), nil
}

// RenderLocalServiceError renders a beautiful error page for local service connection issues
func RenderLocalServiceError(localPort int, errorMessage string) (string, error) {
	if err := Initialize(); err != nil {
		return "", fmt.Errorf("failed to initialize templates: %w", err)
	}

	// Determine error type and customize message
	errorTitle := "Bad Gateway"
	errorCode := "502"
	displayMessage := errorMessage
	var instructions template.HTML

	// Parse error to provide better user experience
	if containsAny(errorMessage, []string{"connection refused", "Failed to connect to local service"}) {
		errorTitle = "Local Service Unavailable"
		errorCode = "502"
		displayMessage = fmt.Sprintf("Could not connect to localhost:%d", localPort)
		instructions = template.HTML(fmt.Sprintf(`
			<h3>💡 To fix this:</h3>
			<ol>
				<li><strong>Start your local service</strong> on port <code>%d</code></li>
				<li>Ensure it's running and accepting connections</li>
				<li>Refresh this page</li>
			</ol>
			<p class="tip">Your tunnel is connected, but your local application isn't running.</p>
		`, localPort))
	} else if containsAny(errorMessage, []string{"timeout"}) {
		errorTitle = "Gateway Timeout"
		errorCode = "504"
		instructions = template.HTML(`
			<h3>💡 What happened:</h3>
			<p>Your local service took too long to respond.</p>
			<p>Please check if your application is running slowly or experiencing issues.</p>
		`)
	}

	data := ErrorPageData{
		Title:        errorTitle,
		ErrorCode:    "Error " + errorCode,
		Message:      displayMessage,
		Instructions: instructions,
	}

	return RenderErrorPage(data)
}

// containsAny checks if a string contains any of the given substrings
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if contains(s, substr) {
			return true
		}
	}
	return false
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
