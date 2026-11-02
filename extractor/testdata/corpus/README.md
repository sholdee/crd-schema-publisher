# Schema validation corpus

End-to-end fixtures for `TestCorpusValidation` in `extractor/corpus_test.go`.
The test converts the inputs with the real extractor and converter code, then
validates every manifest against the generated JSON Schema with the same
validator library kubeconform uses. It exists to catch schemas that are
stricter or looser than the Kubernetes API server, such as a string-only
`Quantity` or a closed `x-kubernetes-preserve-unknown-fields` object.

| Path | Purpose |
| --- | --- |
| `openapi.json` | Trimmed Kubernetes OpenAPI v2 document covering the built-in kinds used below |
| `crds/*.yaml` | CustomResourceDefinitions converted alongside the built-ins |
| `valid/*.yaml` | Manifests the API server accepts; every document must validate |
| `invalid/*.yaml` | Manifests the API server rejects; every document must fail, and a leading `# expect: <text>` comment must appear in the error |

Both `valid/` and `invalid/` files may hold several YAML documents separated
by `---`.

## Regenerating `openapi.json`

`openapi.json` is produced from a real API server document with
`hack/trim-openapi`, which keeps only the named root definitions and their
transitive references and drops descriptions. Regenerate it when adding a
built-in kind to the corpus or when bumping the Kubernetes version it tracks:

```bash
kubectl get --raw /openapi/v2 > swagger.json
go run ./hack/trim-openapi -in swagger.json -out extractor/testdata/corpus/openapi.json \
  -keep "$(tr '\n' ',' < extractor/testdata/corpus/openapi-roots.txt)"
```

`openapi-roots.txt` lists the root definitions, one per line.
