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
			build.OnResolve(api.OnResolveOptions{Filter: `^(react|react/jsx-runtime|react-dom/client|recharts)$`},
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
