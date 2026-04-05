package mcp

import (
	"fmt"
	"regexp"

	"github.com/pkg/browser"
)

// allowedURLPatterns is the allowlist of URL patterns the open_in_browser tool may open.
// Each entry is a compiled regexp; a URL must match at least one to be accepted.
// Extend this slice to permit additional URL shapes in the future.
var allowedURLPatterns = []*regexp.Regexp{
	// http(s)://localhost:<3000-4000>/render-charts/<UUID>
	regexp.MustCompile(`^https?://localhost:(3\d{3}|4000)/render-charts/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
}

func (s *Server) handleOpenInBrowser(url string) (any, error) {
	if url == "" {
		return nil, fmt.Errorf("url must not be empty")
	}
	allowed := false
	for _, re := range allowedURLPatterns {
		if re.MatchString(url) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("URL does not match any allowed pattern and cannot be opened")
	}
	if err := browser.OpenURL(url); err != nil {
		return nil, fmt.Errorf("failed to open browser: %w", err)
	}
	return WrapResponse(
		map[string]string{"status": "opened", "url": url},
		"", 0, nil, nil,
		[]string{"URL opened in default browser."},
	), nil
}
