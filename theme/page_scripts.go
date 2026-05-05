package theme

import (
	_ "embed"
	"strings"
)

//go:embed schema_page.js
var schemaPageJS string

var SchemaPageJS = strings.TrimSuffix(schemaPageJS, "\n")

//go:embed index_page.js
var IndexPageJS string
