package charts

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Payload is the data structure injected into the HTML as window.__MCS_PAYLOAD__.
type Payload struct {
	Data       json.RawMessage `json:"data"`
	Guardrails json.RawMessage `json:"guardrails,omitempty"`
	Workflow   json.RawMessage `json:"workflow"`
}

// RenderChart bundles the JSX template for the given tool with the provided
// payload and returns a self-contained HTML page ready to serve.
func RenderChart(toolName string, payload Payload) (string, error) {
	templateFile, ok := toolTemplates[toolName]
	if !ok {
		return "", fmt.Errorf("no chart template for tool %q", toolName)
	}

	// Read the JSX template from the embedded filesystem.
	templateBytes, err := templatesFS.ReadFile("assets/templates/" + templateFile)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", templateFile, err)
	}

	// Synthesize an entry point that imports the template and mounts it.
	// The template exports a default React component.
	entryPoint := fmt.Sprintf(`
import React from "react";
import { createRoot } from "react-dom/client";
import Chart from "./%s";
createRoot(document.getElementById("root")).render(React.createElement(Chart));
`, templateFile)

	// Bundle with esbuild: transform JSX, resolve vendor shims, produce IIFE.
	result := api.Build(api.BuildOptions{
		Stdin: &api.StdinOptions{
			Contents:   entryPoint,
			ResolveDir: ".",
			Loader:     api.LoaderJSX,
		},
		Bundle:            true,
		Format:            api.FormatIIFE,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		JSX:               api.JSXAutomatic,
		JSXImportSource:   "react",
		Write:             false,
		Plugins: []api.Plugin{
			vendorPlugin(),
			templatePlugin(templateFile, templateBytes),
		},
	})

	if len(result.Errors) > 0 {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Text
		}
		return "", fmt.Errorf("esbuild errors: %s", strings.Join(msgs, "; "))
	}

	if len(result.OutputFiles) == 0 {
		return "", fmt.Errorf("esbuild produced no output")
	}

	bundleJS := string(result.OutputFiles[0].Contents)

	// Serialize the payload for injection.
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	return buildHTML(Title(toolName), string(payloadJSON), string(vendorJS), bundleJS), nil
}

// templatePlugin returns an esbuild plugin that resolves the template file
// import from the synthesized entry point to the embedded JSX content.
func templatePlugin(filename string, content []byte) api.Plugin {
	return api.Plugin{
		Name: "mcs-template",
		Setup: func(build api.PluginBuild) {
			filter := fmt.Sprintf(`\./%s$`, strings.ReplaceAll(filename, ".", `\.`))
			build.OnResolve(api.OnResolveOptions{Filter: filter},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					return api.OnResolveResult{
						Path:      filename,
						Namespace: "mcs-template",
					}, nil
				},
			)
			build.OnLoad(api.OnLoadOptions{Filter: `.*`, Namespace: "mcs-template"},
				func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					c := string(content)
					return api.OnLoadResult{
						Contents: &c,
						Loader:   api.LoaderJSX,
					}, nil
				},
			)
		},
	}
}

// buildHTML assembles a self-contained HTML page from the payload, vendor
// bundle, and the chart-specific bundle.
func buildHTML(title, payloadJSON, vendorScript, chartBundle string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>`)
	b.WriteString(title)
	b.WriteString(`</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  html, body, #root {
    height: 100%; width: 100%;
    background: #080a0f; color: #dde1ef;
    font-family: 'JetBrains Mono', 'Aptos Mono', 'Cascadia Code', Menlo, monospace;
    font-size: 13px;
    -webkit-font-smoothing: antialiased;
  }
  #root { padding: 24px; }
</style>
</head>
<body>
<div id="root"></div>
<script>window.__MCS_PAYLOAD__ = `)
	b.WriteString(payloadJSON)
	b.WriteString(`;</script>
<script>`)
	b.WriteString(vendorScript)
	b.WriteString(`</script>
<script>`)
	b.WriteString(chartBundle)
	b.WriteString(`</script>
</body>
</html>`)
	return b.String()
}
