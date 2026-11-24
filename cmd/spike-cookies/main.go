// Command spike-cookies is a throwaway probe, not part of the product.
//
// Question: can a Jira Data Center session be harvested silently, using the
// machine's own single sign-on, so that pasting cookies out of devtools by
// hand stops being necessary?
//
// It supplies no credentials. A browser is pointed at the Jira dashboard with
// a fresh, empty profile; whatever session that yields comes from the OS-level
// sign-on. The cookies are then fed to the real jira.Client, because a browser
// that works proves nothing about whether authenticateRequest works.
//
// PASS means both halves hold, and automating this is worth building.
//
//	go run ./cmd/spike-cookies -url https://jira.example.com
//
// Delete this directory once the question is answered.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"mcs-mcp/internal/jira"
)

// cookieNames are the five names authenticateRequest sends. Harvesting exactly
// these keeps the spike honest: it proves the production path, not a variant.
var cookieNames = []string{
	"atlassian.xsrf.token",
	"JSESSIONID",
	"seraph.rememberme.cookie",
	"GCILB",
	"GCLB",
}

// defaultEdge is where Edge normally sits on 64-bit Windows. It is a default,
// not a discovery mechanism: -exec overrides it.
const defaultEdge = `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`

func main() {
	base := flag.String("url", os.Getenv("JIRA_URL"), "Jira base URL")
	headful := flag.Bool("headful", false, "show the browser; try this if headless yields no session")
	// Which browser runs is load-bearing. Left to itself chromedp takes the
	// first binary it finds, which is Chrome where both are installed — and
	// Chrome is not wired into the Windows account broker, so it cannot reach
	// the OS sign-on that mints the session.
	exec := flag.String("exec", defaultEdge, "browser executable; Edge is the one wired into Windows sign-on")
	// projects/picker matches on the query and returns nothing for an empty
	// one, so a blank search proves nothing about access.
	query := flag.String("query", "", "project search term; required, the picker ignores an empty query")
	flag.Parse()

	if strings.TrimSpace(*base) == "" {
		fmt.Fprintln(os.Stderr, "need -url or JIRA_URL")
		os.Exit(1)
	}

	if _, err := os.Stat(*exec); err != nil {
		fmt.Fprintf(os.Stderr, "browser not found at %s\n", *exec)
		fmt.Fprintln(os.Stderr, "pass -exec with the path to msedge.exe")
		os.Exit(1)
	}
	fmt.Printf("browser: %s\n\n", *exec)

	jar, err := harvest(*base, *headful, *exec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "harvest failed: %v\n", err)
		os.Exit(1)
	}

	// First half: did a headless browser get a session at all? Values are
	// secrets, so report only their presence and length.
	fmt.Println("== harvested ==")
	for _, name := range cookieNames {
		if v := jar[name]; v != "" {
			fmt.Printf("  %-26s len=%d\n", name, len(v))
		} else {
			fmt.Printf("  %-26s -\n", name)
		}
	}

	if jar["JSESSIONID"] == "" {
		fmt.Fprintln(os.Stderr, "\nFAIL: no JSESSIONID.")
		if !*headful {
			fmt.Fprintln(os.Stderr, "Retry with -headful: conditional access may reject headless.")
		}
		os.Exit(1)
	}

	// Second half, part one: identity. /myself needs no project permissions and
	// names the authenticated user, so it separates "the session is valid" from
	// "the session is valid and is me" — which an empty project list cannot.
	if err := whoAmI(*base, jar); err != nil {
		fmt.Fprintf(os.Stderr, "\nFAIL: %v\n", err)
		os.Exit(1)
	}

	// Second half, part two: does the real client accept them? Deployment is
	// pinned so the probe cannot be blamed on flavour detection.
	client := jira.NewClient(jira.Config{
		BaseURL:    *base,
		Deployment: "datacenter",
		XsrfToken:  jar["atlassian.xsrf.token"],
		SessionID:  jar["JSESSIONID"],
		RememberMe: jar["seraph.rememberme.cookie"],
		GCILB:      jar["GCILB"],
		GCLB:       jar["GCLB"],
	})

	if strings.TrimSpace(*query) == "" {
		fmt.Println("\nPASS: session harvested and authenticated.")
		fmt.Println("Pass -query <term> to also exercise the client's project search.")
		return
	}

	projects, err := client.FindProjects(*query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nFAIL: client rejected the harvested cookies: %v\n", err)
		os.Exit(1)
	}
	if len(projects) == 0 {
		fmt.Fprintf(os.Stderr, "\nFAIL: no projects matched %q — the call succeeded but returned nothing\n", *query)
		os.Exit(1)
	}

	for _, p := range projects {
		if m, ok := p.(map[string]any); ok {
			fmt.Printf("  %v  %v\n", m["key"], m["name"])
		}
	}
	fmt.Printf("\nPASS: %d project(s) fetched with harvested cookies\n", len(projects))
}

// whoAmI calls /rest/api/2/myself with the harvested cookies and reports who
// Jira thinks is calling.
//
// The header is built the way dc_client.authenticateRequest builds it —
// manually, skipping empty values, without net/http's cookie jar, whose RFC
// 6265 validation drops the quoted values Jira and the load balancer emit. A
// probe that assembled it differently would not be testing the real path.
func whoAmI(base string, jar map[string]string) error {
	req, err := http.NewRequest(http.MethodGet,
		strings.TrimRight(base, "/")+"/rest/api/2/myself", nil)
	if err != nil {
		return err
	}

	var pairs []string
	for _, name := range cookieNames {
		if v := jar[name]; v != "" {
			pairs = append(pairs, fmt.Sprintf("%s=%s", name, v))
		}
	}
	req.Header.Set("Cookie", strings.Join(pairs, "; "))
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("myself request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 401 here with cookies present means the browser session does not
		// carry over to a separate HTTP client — the load-balancer stickiness
		// question this spike exists to answer.
		return fmt.Errorf("myself returned %d: session does not transfer to a plain HTTP client",
			resp.StatusCode)
	}

	var me struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Email       string `json:"emailAddress"`
		Active      bool   `json:"active"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return fmt.Errorf("myself returned 200 but not JSON — probably a login page: %w", err)
	}
	if me.Name == "" && me.DisplayName == "" {
		return fmt.Errorf("myself returned 200 with no identity — not an authenticated session")
	}

	fmt.Printf("authenticated as: %s (%s) active=%v\n", me.DisplayName, me.Name, me.Active)
	return nil
}

// harvest opens the Jira dashboard in a throwaway browser profile and returns
// the session cookies the sign-on left behind.
//
// The dashboard is deliberate. A REST path answers 401 outright instead of
// running the redirect that mints the session, so aiming at the API would
// report failure even where sign-on works.
func harvest(base string, headful bool, execPath string) (map[string]string, error) {
	// A fresh profile each run. If a session still appears, it came from the
	// OS rather than from anything cached here — which is the claim under test.
	profile, err := os.MkdirTemp("", "jira-spike-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(profile)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", !headful),
	)
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(alloc)
	defer cancelCtx()

	// Generous: a first sign-on redirect chain is slower than a warm load.
	ctx, cancelTimeout := context.WithTimeout(ctx, 90*time.Second)
	defer cancelTimeout()

	wanted := make(map[string]bool, len(cookieNames))
	for _, n := range cookieNames {
		wanted[n] = true
	}

	jar := make(map[string]string, len(cookieNames))

	// Where the redirect chain ended. An empty harvest is otherwise mute: it
	// cannot say whether sign-on failed or the dashboard was never reached.
	var landed string

	err = chromedp.Run(ctx,
		chromedp.Navigate(strings.TrimRight(base, "/")+"/secure/Dashboard.jspa"),
		chromedp.WaitReady("body"),
		chromedp.Location(&landed),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// JSESSIONID is HttpOnly, so this must come from CDP rather than
			// document.cookie.
			cookies, err := network.GetCookies().Do(ctx)
			if err != nil {
				return err
			}
			for _, c := range cookies {
				if wanted[c.Name] {
					jar[c.Name] = c.Value
				}
			}
			return nil
		}),
	)
	if landed != "" {
		fmt.Printf("landed on: %s\n\n", landed)
	}
	return jar, err
}
