package jira

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// TestIsCloud pins deployment detection to the host. It must not depend on
// TokenType: cookie-authenticated Data Center has no token at all, and routing
// it to the Cloud v3 endpoints produces a 404 on projects that plainly exist.
func TestIsCloud(t *testing.T) {
	tests := []struct {
		baseURL string
		want    bool
	}{
		{"https://acme.atlassian.net", true},
		{"https://acme.atlassian.net/", true},
		{"HTTPS://ACME.ATLASSIAN.NET", true},
		{"  https://acme.atlassian.net  ", true},
		{"https://jira.acme.com", false},
		{"https://jira.internal.acme.corp:8443", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.baseURL, func(t *testing.T) {
			if got := (Config{BaseURL: tc.baseURL}).IsCloud(); got != tc.want {
				t.Errorf("IsCloud(%q) = %v, want %v", tc.baseURL, got, tc.want)
			}
		})
	}
}

// TestRestPathDialect covers the consequence: the resolved deployment picks
// the API version, and the auth method never does. Deployment is set
// explicitly here so the cases stay independent of the network probe.
func TestRestPathDialect(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "data center with cookies uses v2",
			cfg:  Config{BaseURL: "https://jira.acme.com", Deployment: "datacenter", SessionID: "s"},
			want: "https://jira.acme.com/rest/api/2/project/ABC",
		},
		{
			name: "data center with a stray user email still uses v2",
			cfg: Config{
				BaseURL: "https://jira.acme.com", Deployment: "datacenter",
				TokenType: "api", UserEmail: "u@acme.com",
			},
			want: "https://jira.acme.com/rest/api/2/project/ABC",
		},
		{
			name: "cloud on a custom domain uses v3",
			cfg:  Config{BaseURL: "https://jira.acme.com", Deployment: "cloud"},
			want: "https://jira.acme.com/rest/api/3/project/ABC",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &dcClient{cfg: tc.cfg, httpClient: &http.Client{}}
			if got := c.restPath("", "project/ABC"); got != tc.want {
				t.Errorf("restPath = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestResolveIsCloud covers deployment detection end to end: an explicit
// setting wins, otherwise the server is asked, and only an unreachable probe
// falls back to guessing from the host.
func TestResolveIsCloud(t *testing.T) {
	newServer := func(t *testing.T, status int, body string) *httptest.Server {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/rest/api/2/serverInfo" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		return srv
	}

	t.Run("probe reports cloud", func(t *testing.T) {
		srv := newServer(t, http.StatusOK, `{"deploymentType":"Cloud","version":"1001.0.0"}`)
		c := &dcClient{cfg: Config{BaseURL: srv.URL}, httpClient: srv.Client()}
		if !c.resolveIsCloud() {
			t.Error("deploymentType Cloud must resolve to cloud")
		}
	})

	t.Run("probe reports server", func(t *testing.T) {
		srv := newServer(t, http.StatusOK, `{"deploymentType":"Server","version":"11.3.8"}`)
		c := &dcClient{cfg: Config{BaseURL: srv.URL}, httpClient: srv.Client()}
		if c.resolveIsCloud() {
			t.Error("deploymentType Server must resolve to data center")
		}
	})

	t.Run("explicit setting wins over the probe", func(t *testing.T) {
		srv := newServer(t, http.StatusOK, `{"deploymentType":"Server"}`)
		c := &dcClient{
			cfg:        Config{BaseURL: srv.URL, Deployment: "cloud"},
			httpClient: srv.Client(),
		}
		if !c.resolveIsCloud() {
			t.Error("an explicit JIRA_DEPLOYMENT must override the probe")
		}
	})

	t.Run("unreachable probe falls back to the host heuristic", func(t *testing.T) {
		srv := newServer(t, http.StatusForbidden, "")
		// A locked-down instance that refuses serverInfo, on a host that looks
		// like Data Center.
		c := &dcClient{cfg: Config{BaseURL: srv.URL}, httpClient: srv.Client()}
		if c.resolveIsCloud() {
			t.Error("a refused probe on a non-Cloud host must fall back to data center")
		}
	})

	// HTTP does not fix whether auth or routing is checked first, so a 401
	// carries no information about which endpoints exist. It must be reported
	// as a credentials problem, not as a failure to detect the deployment.
	t.Run("rejected credentials are reported as such", func(t *testing.T) {
		srv := newServer(t, http.StatusUnauthorized, "")
		var buf bytes.Buffer
		previous := log.Logger
		log.Logger = zerolog.New(&buf)
		t.Cleanup(func() { log.Logger = previous })

		c := &dcClient{cfg: Config{BaseURL: srv.URL}, httpClient: srv.Client()}
		c.resolveIsCloud()

		if !strings.Contains(buf.String(), "rejected the credentials") {
			t.Errorf("a 401 must be reported as a credentials problem, got: %s", buf.String())
		}
	})

	// An Atlassian host settles the question by itself, so falling back to it
	// is not a guess and must not nag.
	t.Run("conclusive host does not warn", func(t *testing.T) {
		var buf bytes.Buffer
		previous := log.Logger
		log.Logger = zerolog.New(&buf)
		t.Cleanup(func() { log.Logger = previous })

		// No httpClient, so the probe cannot run and the host decides.
		c := &dcClient{cfg: Config{BaseURL: "https://acme.atlassian.net"}}
		if !c.resolveIsCloud() {
			t.Error("an atlassian.net host must resolve to cloud")
		}
		if strings.Contains(buf.String(), `"level":"warn"`) {
			t.Errorf("an unambiguous host must not warn, got: %s", buf.String())
		}
	})

	t.Run("ambiguous host warns that it is guessing", func(t *testing.T) {
		var buf bytes.Buffer
		previous := log.Logger
		log.Logger = zerolog.New(&buf)
		t.Cleanup(func() { log.Logger = previous })

		c := &dcClient{cfg: Config{BaseURL: "https://jira.acme.com"}}
		c.resolveIsCloud()

		if !strings.Contains(buf.String(), "JIRA_DEPLOYMENT") {
			t.Errorf("a custom domain must tell the user how to remove the guess, got: %s", buf.String())
		}
	})

	t.Run("resolved once and cached", func(t *testing.T) {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			_, _ = w.Write([]byte(`{"deploymentType":"Server"}`))
		}))
		t.Cleanup(srv.Close)

		c := &dcClient{cfg: Config{BaseURL: srv.URL}, httpClient: srv.Client()}
		for range 3 {
			c.resolveIsCloud()
		}
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("probe ran %d times, want 1", got)
		}
	})
}

// TestAuthenticateRequest covers which credential wins. The cookie fallback is
// only reachable when no token is set, so a blank-but-present token must not
// suppress it — that combination is what a Claude Desktop install produces
// when the optional token field is left empty.
func TestAuthenticateRequest(t *testing.T) {
	cookies := Config{
		SessionID: "session-value",
		XsrfToken: "xsrf-value",
	}

	tests := []struct {
		name       string
		cfg        Config
		wantAuth   string // expected Authorization header prefix, "" for none
		wantCookie bool
	}{
		{
			name:     "cloud api token uses basic auth",
			cfg:      Config{Token: "tok", TokenType: "api", UserEmail: "user@example.com"},
			wantAuth: "Basic ",
		},
		{
			name:     "data center pat uses bearer",
			cfg:      Config{Token: "tok", TokenType: "pat"},
			wantAuth: "Bearer ",
		},
		{
			name:       "no token falls back to cookies",
			cfg:        cookies,
			wantCookie: true,
		},
		{
			name: "blank token falls back to cookies",
			cfg: Config{
				Token:     "   ",
				TokenType: "pat",
				SessionID: cookies.SessionID,
				XsrfToken: cookies.XsrfToken,
			},
			wantCookie: true,
		},
		{
			name: "token wins over cookies",
			cfg: Config{
				Token:     "tok",
				TokenType: "pat",
				SessionID: cookies.SessionID,
			},
			wantAuth: "Bearer ",
		},
		{
			name: "no credentials sets nothing",
			cfg:  Config{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &dcClient{cfg: tc.cfg}
			req, err := http.NewRequest(http.MethodGet, "https://jira.example.com", nil)
			if err != nil {
				t.Fatal(err)
			}

			c.authenticateRequest(req)

			auth := req.Header.Get("Authorization")
			switch {
			case tc.wantAuth == "" && auth != "":
				t.Errorf("Authorization = %q, want none", auth)
			case tc.wantAuth != "" && !strings.HasPrefix(auth, tc.wantAuth):
				t.Errorf("Authorization = %q, want prefix %q", auth, tc.wantAuth)
			}

			if got := req.Header.Get("Cookie") != ""; got != tc.wantCookie {
				t.Errorf("cookie header present = %v, want %v", got, tc.wantCookie)
			}
		})
	}
}
