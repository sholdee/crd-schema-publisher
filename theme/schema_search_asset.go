package theme

import (
	_ "embed"
	"os"
	"path/filepath"
)

const SchemaSearchAssetName = "schema-search.js"

//go:embed schema_search.js
var SchemaSearchJS string

func WriteSchemaSearchAsset(outputDir string) error {
	return os.WriteFile(filepath.Join(outputDir, SchemaSearchAssetName), []byte(SchemaSearchJS), 0o644)
}
