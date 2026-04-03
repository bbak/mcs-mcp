#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
npm install
npx esbuild --bundle vendor_entry.js --outfile=vendor.js --format=iife --minify --define:process.env.NODE_ENV=\"production\"
echo "vendor.js rebuilt successfully ($(wc -c < vendor.js) bytes)"
