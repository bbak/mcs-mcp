#!/usr/bin/env python3
"""
inject.py — MCS Charts placeholder injector

Usage:
  python3 inject.py <jsx_path> <mcp_response_json_path> [chart_attrs_json_path]

Replaces "__MCP_RESPONSE__" and "__CHART_ATTRS__" in the JSX file in-place.
If chart_attrs_json_path is omitted, "__CHART_ATTRS__" is replaced with {}.
"""

import sys
import pathlib

def main():
    if len(sys.argv) < 3:
        print("Usage: inject.py <jsx_path> <mcp_response_json> [chart_attrs_json]", file=sys.stderr)
        sys.exit(1)

    jsx_path   = pathlib.Path(sys.argv[1])
    mcp_path   = pathlib.Path(sys.argv[2])
    attrs_path = pathlib.Path(sys.argv[3]) if len(sys.argv) >= 4 else None

    mcp_json   = mcp_path.read_text(encoding="utf-8")
    attrs_json = attrs_path.read_text(encoding="utf-8") if attrs_path else "{}"

    content = jsx_path.read_text(encoding="utf-8")
    content = content.replace('"__MCP_RESPONSE__"', mcp_json)
    content = content.replace('"__CHART_ATTRS__"',  attrs_json)
    jsx_path.write_text(content, encoding="utf-8")

    print(f"Injected: {jsx_path}")

if __name__ == "__main__":
    main()
