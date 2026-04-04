package charts

import (
	"github.com/evanw/esbuild/pkg/api"
)

// vendorShims maps bare import specifiers to JavaScript shim code that
// re-exports from the pre-bundled window.__MCS_VENDOR__ globals.
var vendorShims = map[string]string{
	"react": `
		var V = window.__MCS_VENDOR__.React;
		export default V;
		export var useState = V.useState;
		export var useMemo = V.useMemo;
		export var useCallback = V.useCallback;
		export var useEffect = V.useEffect;
		export var useRef = V.useRef;
		export var Fragment = V.Fragment;
		export var createElement = V.createElement;
	`,
	"react/jsx-runtime": `
		var V = window.__MCS_VENDOR__.React;
		export var jsx = V.createElement;
		export var jsxs = V.createElement;
		export var Fragment = V.Fragment;
	`,
	"react-dom/client": `
		export var createRoot = window.__MCS_VENDOR__.ReactDOM.createRoot;
	`,
	"recharts": `
		var R = window.__MCS_VENDOR__.Recharts;
		export var AreaChart = R.AreaChart;
		export var Area = R.Area;
		export var BarChart = R.BarChart;
		export var Bar = R.Bar;
		export var Cell = R.Cell;
		export var CartesianGrid = R.CartesianGrid;
		export var ComposedChart = R.ComposedChart;
		export var Legend = R.Legend;
		export var Line = R.Line;
		export var LineChart = R.LineChart;
		export var Pie = R.Pie;
		export var PieChart = R.PieChart;
		export var ReferenceLine = R.ReferenceLine;
		export var ReferenceArea = R.ReferenceArea;
		export var ResponsiveContainer = R.ResponsiveContainer;
		export var Scatter = R.Scatter;
		export var ScatterChart = R.ScatterChart;
		export var Tooltip = R.Tooltip;
		export var XAxis = R.XAxis;
		export var YAxis = R.YAxis;
		export var ZAxis = R.ZAxis;
		export var Customized = R.Customized;
	`,
	"mcs-mcp": `
		export var ALARM     = "#ff6b6b";
		export var CAUTION   = "#e2c97e";
		export var PRIMARY   = "#6b7de8";
		export var SECONDARY = "#7edde2";
		export var TERTIARY  = "#c084fc";
		export var POSITIVE  = "#6bffb8";
		export var TEXT      = "#dde1ef";
		export var MUTED     = "#505878";
		export var PAGE_BG   = "#080a0f";
		export var PANEL_BG  = "#1d1d2c";
		export var BORDER    = "#252a42";

		var ISSUE_TYPE_PALETTE = [
			"#6b7de8","#ff6b6b","#7edde2","#e2c97e",
			"#53fdab","#ef863a","#8b5cf6","#f43e99"
		];

		export function typeColor(name, allTypes) {
			var sorted = allTypes.slice().sort();
			var idx    = sorted.indexOf(name);
			return ISSUE_TYPE_PALETTE[(idx < 0 ? 0 : idx) % ISSUE_TYPE_PALETTE.length];
		}

		// Typography — matches the Go-side font stack in render.go.
		export var FONT_STACK = "'JetBrains Mono', 'Aptos Mono', 'Cascadia Code', Menlo, monospace";

		// XmR reference line colors.
		export var XMR_UNPL = ALARM;    // upper natural process limit  → red
		export var XMR_MEAN = TERTIARY; // process mean (X̄)             → purple
		export var XMR_LNPL = POSITIVE; // lower natural process limit  → green

		// Percentile color mapping — stable because the percentile slots are fixed.
		// Reads as a risk gradient: green (fast/safe) → teal → purple → blue → amber → red (slow/risky).
		var PERC_COLOR_MAP = {
			aggressive:     POSITIVE,  // P10
			unlikely:       POSITIVE,  // P30
			coin_toss:      SECONDARY, // P50
			probable:       TERTIARY,  // P70
			likely:         PRIMARY,   // P85 — canonical SLE
			conservative:   CAUTION,   // P90
			safe:           ALARM,     // P95
			almost_certain: ALARM,     // P98
		};

		export function percColor(key) {
			return PERC_COLOR_MAP[key] || MUTED;
		}

		// Workflow tier colors — stable domain: Demand, Upstream, Downstream.
		export function tierColor(tier) {
			if (tier === "Demand")     return CAUTION;   // amber  — demand pressure
			if (tier === "Upstream")   return PRIMARY;   // blue   — upstream flow
			if (tier === "Downstream") return SECONDARY; // teal   — downstream delivery
			return MUTED;
		}

		// Severity colors — stable domain: Low, Medium, High.
		export function severityColor(severity) {
			if (severity === "Low")    return POSITIVE; // green
			if (severity === "Medium") return CAUTION;  // amber
			if (severity === "High")   return ALARM;    // red
			return MUTED;
		}
	`,
}

// vendorPlugin returns an esbuild plugin that intercepts bare specifier
// imports (react, recharts) and resolves them to shims that re-export
// from the pre-bundled window.__MCS_VENDOR__ globals.
func vendorPlugin() api.Plugin {
	return api.Plugin{
		Name: "mcs-vendor-shim",
		Setup: func(build api.PluginBuild) {
			// Intercept bare specifiers.
			build.OnResolve(api.OnResolveOptions{Filter: `^(react|react/jsx-runtime|react-dom/client|recharts|mcs-mcp)$`},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					return api.OnResolveResult{
						Path:      args.Path,
						Namespace: "vendor-shim",
					}, nil
				},
			)

			// Return the shim code for each resolved specifier.
			build.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: "vendor-shim"},
				func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					contents := vendorShims[args.Path]
					return api.OnLoadResult{
						Contents: &contents,
						Loader:   api.LoaderJS,
					}, nil
				},
			)
		},
	}
}
