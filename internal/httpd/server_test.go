package httpd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"mcs-mcp/internal/chartbuf"
	"mcs-mcp/internal/charts"
)

func stubRenderer(_ string, _ charts.Payload) (string, error) {
	return "<html><body>chart</body></html>", nil
}

func startTestServer(t *testing.T, bufSize int, renderer RenderFunc) (*Server, func()) {
	t.Helper()
	buf := chartbuf.NewBuffer(bufSize)
	srv, err := New(buf, renderer)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = srv.Start(ctx)
	}()

	// Give the server a moment to start.
	time.Sleep(20 * time.Millisecond)

	return srv, cancel
}

func TestNotFound(t *testing.T) {
	srv, cancel := startTestServer(t, 5, stubRenderer)
	defer cancel()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/render-charts/nonexistent", srv.Port()))
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got status %d, want 404", resp.StatusCode)
	}
}

func TestRenderSuccess(t *testing.T) {
	srv, cancel := startTestServer(t, 5, stubRenderer)
	defer cancel()

	// Push a test entry.
	envelope := map[string]any{
		"data":       map[string]any{"x": 1},
		"guardrails": map[string]any{"warnings": []string{}},
	}
	envelopeJSON, _ := json.Marshal(envelope)
	workflow := json.RawMessage(`{"board_id": 1}`)
	uuid := srv.buf.Push("analyze_throughput", envelopeJSON, workflow)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/render-charts/%s", srv.Port(), uuid))
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("got Content-Type %q, want text/html; charset=utf-8", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<html><body>chart</body></html>" {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestCaching(t *testing.T) {
	callCount := 0
	countingRenderer := func(_ string, _ charts.Payload) (string, error) {
		callCount++
		return "<html>cached</html>", nil
	}

	srv, cancel := startTestServer(t, 5, countingRenderer)
	defer cancel()

	envelope, _ := json.Marshal(map[string]any{"data": map[string]any{}})
	uuid := srv.buf.Push("analyze_throughput", envelope, json.RawMessage(`{}`))

	url := fmt.Sprintf("http://127.0.0.1:%d/render-charts/%s", srv.Port(), uuid)

	// First request: renders.
	resp, _ := http.Get(url)
	resp.Body.Close()

	// Second request: should be cached.
	resp, _ = http.Get(url)
	resp.Body.Close()

	if callCount != 1 {
		t.Errorf("renderer called %d times, want 1 (should be cached)", callCount)
	}
}
