package charts

import "embed"

//go:embed assets/templates/*.jsx
var templatesFS embed.FS

//go:embed assets/vendor.js
var vendorJS []byte
