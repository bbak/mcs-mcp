// Package httpd runs a lightweight localhost HTTP server that serves rendered
// chart pages from chartbuf via UUID lookup. Binds to a random port in
// [3000, 4000] alongside the stdio MCP transport.
package httpd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"

	"mcs-mcp/internal/chartbuf"
	"mcs-mcp/internal/charts"

	"github.com/rs/zerolog/log"
)

// RenderFunc is the function signature for chart rendering.
type RenderFunc func(toolName string, payload charts.Payload) (string, error)

// Server serves rendered charts over HTTP on localhost.
type Server struct {
	buf      *chartbuf.Buffer
	renderer RenderFunc
	listener net.Listener
	srv      *http.Server

	// htmlCache caches rendered HTML by UUID to avoid re-rendering.
	cacheMu   sync.Mutex
	htmlCache map[string]string
}

// New creates a chart HTTP server bound to a random free port in 3000-4000.
func New(buf *chartbuf.Buffer, renderer RenderFunc) (*Server, error) {
	ln, err := findFreePort(3000, 4000)
	if err != nil {
		return nil, err
	}

	s := &Server{
		buf:       buf,
		renderer:  renderer,
		listener:  ln,
		htmlCache: make(map[string]string),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /render-charts/{uuid}", s.handleRenderChart)

	s.srv = &http.Server{Handler: mux}
	return s, nil
}

// Port returns the port the server is listening on.
func (s *Server) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// Start begins serving HTTP requests. It blocks until ctx is cancelled,
// then gracefully shuts down the server.
func (s *Server) Start(ctx context.Context) error {
	log.Info().Int("port", s.Port()).Msg("Chart HTTP server listening")

	errCh := make(chan error, 1)
	go func() {
		if err := s.srv.Serve(s.listener); err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		log.Info().Msg("Chart HTTP server shutting down")
		return s.srv.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleRenderChart(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		http.Error(w, "missing chart UUID", http.StatusBadRequest)
		return
	}

	// Check HTML cache first.
	s.cacheMu.Lock()
	cached, hit := s.htmlCache[uuid]
	s.cacheMu.Unlock()
	if hit {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, cached)
		return
	}

	// Look up the entry in the MRU buffer.
	entry, ok := s.buf.Get(uuid)
	if !ok {
		http.Error(w, "chart not found — it may have been evicted from the buffer", http.StatusNotFound)
		return
	}

	// Extract the ResponseEnvelope's data and guardrails for the payload.
	var envelope struct {
		Data       json.RawMessage `json:"data"`
		Guardrails json.RawMessage `json:"guardrails"`
	}
	if err := json.Unmarshal(entry.Data, &envelope); err != nil {
		log.Error().Err(err).Str("uuid", uuid).Msg("Failed to unmarshal stored envelope")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	payload := charts.Payload{
		Data:       envelope.Data,
		Guardrails: envelope.Guardrails,
		Workflow:   entry.Workflow,
	}

	html, err := s.renderer(entry.ToolName, payload)
	if err != nil {
		log.Error().Err(err).Str("tool", entry.ToolName).Str("uuid", uuid).Msg("Chart render failed")
		http.Error(w, "render error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Cache the result.
	s.cacheMu.Lock()
	s.htmlCache[uuid] = html
	s.cacheMu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

// findFreePort finds the first available TCP port in [low, high] on localhost.
func findFreePort(low, high int) (net.Listener, error) {
	for port := low; port <= high; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("no free port found in range %d-%d", low, high)
}
