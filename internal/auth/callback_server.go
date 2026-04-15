package auth

import (
	"fmt"
	"html"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	defaultPort    = 17310
	maxPort        = 17319
	successHTML    = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>AIToll CLI - Login Successful</title></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f5f5f5;">
<div style="text-align: center; padding: 40px; background: white; border-radius: 12px; box-shadow: 0 2px 8px rgba(0,0,0,0.1);">
<h1 style="color: #22c55e; margin-bottom: 8px;">&#10003; Login Successful</h1>
<p style="color: #666; margin: 0;">You can close this tab and return to the terminal.</p>
</div>
</body>
</html>`
	errorHTML = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>AIToll CLI - Login Failed</title></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f5f5f5;">
<div style="text-align: center; padding: 40px; background: white; border-radius: 12px; box-shadow: 0 2px 8px rgba(0,0,0,0.1);">
<h1 style="color: #ef4444; margin-bottom: 8px;">&#10007; Login Failed</h1>
<p style="color: #666; margin: 0;">%s</p>
<p style="color: #999; font-size: 14px; margin-top: 16px;">You can close this tab and try again from the terminal.</p>
</div>
</body>
</html>`
)

// CallbackServer manages the temporary local HTTP server for OAuth callback.
type CallbackServer struct {
	server  *http.Server
	state   string
	tokenCh chan string
	errCh   chan error
	mu      sync.Mutex
	done    bool
}

// NewCallbackServer creates a new CallbackServer with the expected state value.
func NewCallbackServer(state string) *CallbackServer {
	return &CallbackServer{
		state:   state,
		tokenCh: make(chan string, 1),
		errCh:   make(chan error, 1),
	}
}

// Start begins listening on the given port. Returns the actual listen address.
func (s *CallbackServer) Start(port int) (string, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", s.handleCallback)

	var lastErr error
	for p := port; p <= maxPort; p++ {
		addr := fmt.Sprintf("127.0.0.1:%d", p)
		s.server = &http.Server{Handler: mux}

		listener, err := net.Listen("tcp", addr)
		if err != nil {
			lastErr = err
			continue
		}

		go func() {
			if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
				s.mu.Lock()
				if !s.done {
					s.errCh <- err
				}
				s.mu.Unlock()
			}
		}()

		return fmt.Sprintf("http://127.0.0.1:%d", p), nil
	}

	return "", fmt.Errorf("failed to start callback server on ports %d-%d: %w", port, maxPort, lastErr)
}

// WaitForResult blocks until a token is received, an error occurs, or timeout.
func (s *CallbackServer) WaitForResult(timeout time.Duration) (string, error) {
	select {
	case token := <-s.tokenCh:
		return token, nil
	case err := <-s.errCh:
		return "", err
	case <-time.After(timeout):
		return "", fmt.Errorf("timed out waiting for login callback ( waited %v )", timeout)
	}
}

// Stop shuts down the callback server.
func (s *CallbackServer) Stop() {
	s.mu.Lock()
	s.done = true
	s.mu.Unlock()

	if s.server != nil {
		s.server.Close()
	}
}

func (s *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	state := r.URL.Query().Get("state")

	if state == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, errorHTML, "Missing state parameter")
		return
	}

	if state != s.state {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, errorHTML, "State mismatch - possible CSRF attack")
		return
	}

	if token == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, errorHTML, "Missing token parameter")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(successHTML))

	s.tokenCh <- html.UnescapeString(token)
}
