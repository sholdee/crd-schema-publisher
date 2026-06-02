package schemametadata

const (
	MetadataDirName            = "_meta"
	KindsManifestName          = "kinds.json"
	SchemaMetadataManifestName = "schema-metadata.json"
)

type SchemaSource string

const (
	SchemaSourceCRD       SchemaSource = "crd"
	SchemaSourceBuiltin   SchemaSource = "builtin"
	SchemaSourceKustomize SchemaSource = "kustomize"
)

type SchemaMetadataEntry struct {
	Kind   string       `json:"kind"`
	Source SchemaSource `json:"source"`
}
