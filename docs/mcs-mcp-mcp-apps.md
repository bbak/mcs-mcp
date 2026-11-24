# MCP Apps — Decision Record

**Status:** Not pursued. Decided 2026-09-06.
**Supersedes:** the former `mcs-mcp-mcp-apps-design-brief.md` (design brief + spike findings, 2026-05-04).

## What was explored

Delivering flow analytics charts as an MCP App (SEP-1865, `io.modelcontextprotocol/ui`) — a `ui://`
resource rendered inline in the host's sandboxed iframe — instead of the current localhost
`chart_url` the user opens in a browser.

A throwaway spike was built against Claude Desktop. It is not in the tree.

## Why not

**Inline space contradicts the reports we actually build.** Claude Desktop caps inline iframe height
at roughly 720 px (measured in the spike). The full dashboards — `cycle_time.jsx` is ~6 panels: stat
cards, predictability, scatter, pool SLE, per-type SLE, attainment trend, breach severity — do not
fit and cannot be made to fit.

The conformant answer is a *separate slim template* per chart: one primary visual, a compact header,
and an "Open full ↗" link back to the localhost dashboard. So MCP Apps yields a thumbnail that links
to the artifact we already ship, plus a second set of templates to maintain.

Our reports are moving the other way — they render full-screen and would benefit from *more* canvas,
not less. The cost/benefit is inverted, so the migration is not worth its price today.

## Spike findings worth keeping

Empirical results, not restatements of the spec. Verified against SEP-1865 (2026-01-26) after the fact.

1. **`_meta.ui.resourceUri` must sit on the tool descriptor, read at `tools/list` time — not on
   `CallToolResult`.** The spike registered a per-run resource and stamped the call result; the
   server side worked and logged, and Claude Desktop *never* issued `resources/read`. Per-run
   dynamic UI resources are a dead end.
2. **Hosts cache the UI resource by URL.** URL identity means template identity. Per-call URL
   mutation either fails outright or defeats the caching design. Static template + data over
   postMessage is the only conformant shape.
3. **Inline iframe height caps at ~720 px in Claude Desktop.** The measurement behind the decision
   above. Note the 2026-07-28 spec since added negotiated `containerDimensions` on `HostContext`
   (fixed vs. flexible sizing) — worth re-measuring before trusting this number again.
4. **`CallToolResult.StructuredContent` is the field forwarded to the iframe.** `Content[0].Text`
   is agent-facing only. MCS-MCP leaves `StructuredContent` unset; any future wiring must change that.
5. **`ui/open-link` needs no CSP entries.** It is gated by host capability, not by
   `_meta.ui.csp.connectDomains`. CSP only matters for `fetch()`/asset loads from inside the iframe.
   The localhost server stays reachable from an inline view without CSP work.

## What changed in the ecosystem since the brief (checked 2026-09-06)

The original brief is written against the 2026-01-26 spec and is stale in these respects:

- **MCP 2026-07-28 shipped.** The `initialize` handshake is gone, replaced by a required
  `server/discover` RPC. Extension capabilities now negotiate via per-request
  `_meta["io.modelcontextprotocol/clientCapabilities"]` and the `server/discover` response — not the
  `initialize` result the brief describes.
- **The core went stateless.** Sessions and `Mcp-Session-Id` are gone; state must travel as explicit
  handles in tool arguments. This directly invalidates the brief's inline data contract, which had
  the iframe call with empty `arguments` and the server resolving against `s.activeSourceID`.
- **The Go SDK caught up.** We are on go-sdk v1.7.0. `ClientCapabilities`/`ServerCapabilities` carry
  `Extensions` with `AddExtension()` (issue #777, PR #794). Typed `_meta.ui` shapes and
  `registerAppTool`-style sugar are still absent, but the brief's "no Go library supports this"
  framing no longer holds — and neither does its proposal to build and upstream a wrapper package.
- **Push works; the brief's pull-only claim was wrong.** The spec requires the host to send
  `ui/notifications/tool-result`, and the current SDK pattern sets `app.ontoolresult` *before*
  `app.connect()`, using `callServerTool()` only for user-driven refresh. The spike most likely hit
  that ordering requirement rather than a broken push.
- **Host support is broad**, not Claude Desktop only: Claude web and Desktop, VS Code Copilot,
  M365 Copilot, ChatGPT, Cursor, Goose, Postman, MCPJam, Archestra, PostHog.
- **New spec surface** the brief predates: `ui/notifications/tool-input`, `ui/resource-teardown`,
  `ui/message`, `ui/update-model-context`, and `visibility: ["app"]` for tools hidden from the model
  but callable by the View.

## Revisit when

- Hosts offer a materially larger inline canvas, or a way to expand an app to full width/height.
- Or the value shifts from *rendering* to *interaction* — a view whose controls call back into
  the server is worth an iframe in a way a static thumbnail is not.

Re-verify against the then-current spec before reusing anything above; the delta over four months
was already large.

## Carried forward separately

The brief bundled one improvement that never depended on MCP Apps: splitting the dense per-issue
chart payload from a slim analytical tool result. That idea is logged in `docs/ideas.md` under
"Slim the agent-facing tool result" and remains open.

## References

- MCP Apps overview: https://modelcontextprotocol.io/extensions/apps/overview
- Spec (SEP-1865): https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx
- Extension negotiation: https://modelcontextprotocol.io/extensions/overview
- Client matrix: https://modelcontextprotocol.io/extensions/client-matrix
