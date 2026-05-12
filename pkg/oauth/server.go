package oauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// CallbackServer handles OAuth redirect callbacks via a local HTTP server.
type CallbackServer struct {
	port     int
	server   *http.Server
	listener net.Listener
	state    string
	resultCh chan OAuthCallbackData
	cancelCh chan struct{}
}

// NewCallbackServer creates a new callback server starting at the given port.
func NewCallbackServer(startPort int) *CallbackServer {
	if startPort <= 0 {
		startPort = 8311
	}
	return &CallbackServer{
		port:     startPort,
		resultCh: make(chan OAuthCallbackData, 1),
		cancelCh: make(chan struct{}),
	}
}

// SetState sets the expected state for CSRF protection.
func (cs *CallbackServer) SetState(state string) {
	cs.state = state
}

// Start starts the HTTP server and returns the actual listening port.
func (cs *CallbackServer) Start() (int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", cs.handleCallback)

	for {
		addr := fmt.Sprintf("127.0.0.1:%d", cs.port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok {
				if sysErr, ok := opErr.Err.(*net.AddrError); ok && sysErr.Err == "address already in use" {
					cs.port++
					continue
				}
			}
			// Check if port is in use by trying to detect the error string
			if isAddrInUse(err) {
				cs.port++
				continue
			}
			return 0, err
		}
		cs.listener = ln
		break
	}

	cs.server = &http.Server{
		Handler: mux,
	}

	go func() {
		_ = cs.server.Serve(cs.listener)
	}()

	return cs.port, nil
}

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "address already in use") ||
		contains(err.Error(), "Only one usage of each socket address")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Stop shuts down the callback server.
func (cs *CallbackServer) Stop() error {
	close(cs.cancelCh)
	if cs.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return cs.server.Shutdown(ctx)
	}
	return nil
}

// ResultChannel returns the channel that receives callback data.
func (cs *CallbackServer) ResultChannel() <-chan OAuthCallbackData {
	return cs.resultCh
}

// GetURL returns the callback URL.
func (cs *CallbackServer) GetURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/", cs.port)
}

func (cs *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	data := OAuthCallbackData{
		Code:             query.Get("code"),
		Token:            query.Get("token"),
		State:            query.Get("state"),
		Error:            query.Get("error"),
		ErrorDescription: query.Get("error_description"),
	}

	// Validate state if set
	if cs.state != "" && data.State != cs.state {
		data.Error = "invalid_state"
		data.ErrorDescription = "CSRF state mismatch"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if data.Error != "" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>OAuth Callback</title></head>
<body style="display:flex;justify-content:center;align-items:center;height:100vh;font-family:sans-serif">
  <div style="text-align:center">
    <h1 style="color:red">Login Failed</h1>
    <p>%s</p>
    <p style="font-size:12px;opacity:0.7">This window can be closed</p>
  </div>
  <script>setTimeout(()=>window.close(),2000);</script>
</body>
</html>`, data.ErrorDescription)))
	} else {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>OAuth Callback</title></head>
<body style="display:flex;justify-content:center;align-items:center;height:100vh;font-family:sans-serif">
  <div style="text-align:center">
    <h1 style="color:green">Login Successful</h1>
    <p>Processing, please wait...</p>
    <p style="font-size:12px;opacity:0.7">This window can be closed</p>
  </div>
  <script>setTimeout(()=>window.close(),2000);</script>
</body>
</html>`))
	}

	select {
	case cs.resultCh <- data:
	case <-time.After(100 * time.Millisecond):
	}
}
