package extractor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const singleCRDYAML = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: certificates.cert-manager.io
spec:
  group: cert-manager.io
  names:
    kind: Certificate
    plural: certificates
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
`

const multiDocYAML = `---
apiVersion: v1
kind: Namespace
metadata:
  name: test
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: certificates.cert-manager.io
spec:
  group: cert-manager.io
  names:
    kind: Certificate
    plural: certificates
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: issuers.cert-manager.io
spec:
  group: cert-manager.io
  names:
    kind: Issuer
    plural: issuers
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
`

const crdListYAML = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinitionList
items:
  - apiVersion: apiextensions.k8s.io/v1
    kind: CustomResourceDefinition
    metadata:
      name: certificates.cert-manager.io
    spec:
      group: cert-manager.io
      names:
        kind: Certificate
        plural: certificates
      scope: Namespaced
      versions:
        - name: v1
          served: true
          storage: true
          schema:
            openAPIV3Schema:
              type: object
  - apiVersion: apiextensions.k8s.io/v1
    kind: CustomResourceDefinition
    metadata:
      name: issuers.cert-manager.io
    spec:
      group: cert-manager.io
      names:
        kind: Issuer
        plural: issuers
      scope: Namespaced
      versions:
        - name: v1
          served: true
          storage: true
          schema:
            openAPIV3Schema:
              type: object
`

const kubectlCRDListYAML = `apiVersion: v1
items:
  - apiVersion: apiextensions.k8s.io/v1
    kind: CustomResourceDefinition
    metadata:
      name: certificates.cert-manager.io
    spec:
      group: cert-manager.io
      names:
        kind: Certificate
        plural: certificates
      scope: Namespaced
      versions:
        - name: v1
          served: true
          storage: true
          schema:
            openAPIV3Schema:
              type: object
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: ignored
  - apiVersion: apiextensions.k8s.io/v1
    kind: CustomResourceDefinition
    metadata:
      name: issuers.cert-manager.io
    spec:
      group: cert-manager.io
      names:
        kind: Issuer
        plural: issuers
      scope: Namespaced
      versions:
        - name: v1
          served: true
          storage: true
          schema:
            openAPIV3Schema:
              type: object
kind: List
metadata:
  resourceVersion: ""
`

func TestParseCRDsFromReader_SingleCRD(t *testing.T) {
	crds, err := ParseCRDsFromReader(strings.NewReader(singleCRDYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 1 {
		t.Fatalf("expected 1 CRD, got %d", len(crds))
	}
	if crds[0].Spec.Names.Kind != "Certificate" {
		t.Errorf("expected Certificate, got %s", crds[0].Spec.Names.Kind)
	}
}

func TestParseCRDsFromReader_MultiDocSkipsNonCRDs(t *testing.T) {
	crds, err := ParseCRDsFromReader(strings.NewReader(multiDocYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 2 {
		t.Fatalf("expected 2 CRDs (Namespace skipped), got %d", len(crds))
	}
	kinds := map[string]bool{}
	for _, crd := range crds {
		kinds[crd.Spec.Names.Kind] = true
	}
	if !kinds["Certificate"] || !kinds["Issuer"] {
		t.Errorf("expected Certificate and Issuer, got %v", kinds)
	}
}

func TestParseCRDsFromReader_CustomResourceDefinitionList(t *testing.T) {
	crds, err := ParseCRDsFromReader(strings.NewReader(crdListYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 2 {
		t.Fatalf("expected 2 CRDs from list, got %d", len(crds))
	}
	kinds := map[string]bool{}
	for _, crd := range crds {
		kinds[crd.Spec.Names.Kind] = true
	}
	if !kinds["Certificate"] || !kinds["Issuer"] {
		t.Errorf("expected Certificate and Issuer, got %v", kinds)
	}
}

func TestParseCRDsFromReader_KubectlGenericList(t *testing.T) {
	crds, err := ParseCRDsFromReader(strings.NewReader(kubectlCRDListYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 2 {
		t.Fatalf("expected 2 CRDs from kubectl list, got %d", len(crds))
	}
	kinds := map[string]bool{}
	for _, crd := range crds {
		kinds[crd.Spec.Names.Kind] = true
	}
	if !kinds["Certificate"] || !kinds["Issuer"] {
		t.Errorf("expected Certificate and Issuer, got %v", kinds)
	}
}

func TestParseCRDsFromReader_MalformedYAML_FailsFast(t *testing.T) {
	malformed := "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nspec: [invalid"
	_, err := ParseCRDsFromReader(strings.NewReader(malformed))
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestParseCRDsFromReader_EmptyInput(t *testing.T) {
	crds, err := ParseCRDsFromReader(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 0 {
		t.Fatalf("expected 0 CRDs, got %d", len(crds))
	}
}

func TestParseCRDsFromReader_EmptyDocuments(t *testing.T) {
	crds, err := ParseCRDsFromReader(strings.NewReader("---\n---\n---\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 0 {
		t.Fatalf("expected 0 CRDs from empty documents, got %d", len(crds))
	}
}

func TestParseCRDsFromReader_LongLines(t *testing.T) {
	// CRDs with long lines (e.g., base64 blobs, deeply nested schemas) should
	// parse correctly. The old bufio.Scanner had a 64KB line limit.
	longValue := strings.Repeat("a", 128*1024) // 128KB single line
	crd := fmt.Sprintf(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tests.example.io
  annotations:
    big-blob: "%s"
spec:
  group: example.io
  names:
    kind: Test
    plural: tests
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
`, longValue)

	crds, err := ParseCRDsFromReader(strings.NewReader(crd))
	if err != nil {
		t.Fatalf("expected long-line CRD to parse, got: %v", err)
	}
	if len(crds) != 1 {
		t.Fatalf("expected 1 CRD, got %d", len(crds))
	}
}

func TestParseCRDsFromReader_DocumentCountLimit(t *testing.T) {
	var buf strings.Builder
	for range maxDocuments + 1 {
		buf.WriteString("---\nkind: ConfigMap\n")
	}
	_, err := ParseCRDsFromReader(strings.NewReader(buf.String()))
	if err == nil {
		t.Fatal("expected error for too many documents")
	}
	if !strings.Contains(err.Error(), "documents") {
		t.Errorf("expected document count error, got: %v", err)
	}
}

func TestParseCRDsFromReader_ExactlyMaxDocuments(t *testing.T) {
	var buf strings.Builder
	for range maxDocuments {
		buf.WriteString("---\nkind: ConfigMap\n")
	}
	// Should succeed — exactly at the limit, not over
	_, err := ParseCRDsFromReader(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("expected %d documents to be accepted, got: %v", maxDocuments, err)
	}
}

func TestParseCRDsFromReader_InputSizeLimit(t *testing.T) {
	// Build input that exceeds maxInputSize: a valid CRD followed by padding
	padding := strings.Repeat("# padding\n", maxInputSize/10+1)
	input := singleCRDYAML + padding

	_, err := ParseCRDsFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for oversized input")
	}
	if !strings.Contains(err.Error(), "maximum size") {
		t.Errorf("expected size limit error, got: %v", err)
	}
}

func TestParseCRDsFromReader_SeparatorWithTrailingWhitespace(t *testing.T) {
	// YAML spec allows "--- " with trailing spaces as a document separator
	input := "---   \napiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: tests.example.io\nspec:\n  group: example.io\n  names:\n    kind: Test\n    plural: tests\n  scope: Namespaced\n  versions:\n    - name: v1\n      served: true\n      storage: true\n      schema:\n        openAPIV3Schema:\n          type: object\n"
	crds, err := ParseCRDsFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 1 {
		t.Fatalf("expected 1 CRD, got %d", len(crds))
	}
}

func TestParseCRDsFromFiles_ReadsMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cert.yaml"), []byte(singleCRDYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "multi.yaml"), []byte(multiDocYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	crds, err := ParseCRDsFromFiles([]string{
		filepath.Join(dir, "cert.yaml"),
		filepath.Join(dir, "multi.yaml"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 3 {
		t.Fatalf("expected 3 CRDs total, got %d", len(crds))
	}
}

func TestParseCRDsFromFiles_NonexistentFile_Fails(t *testing.T) {
	_, err := ParseCRDsFromFiles([]string{"/nonexistent/crd.yaml"})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestParseCRDsFromDir_ReadsYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "crd.yaml"), []byte(singleCRDYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "crd2.yml"), []byte(singleCRDYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	crds, err := ParseCRDsFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 2 {
		t.Fatalf("expected 2 CRDs (.yaml + .yml), got %d", len(crds))
	}
}

func TestParseCRDsFromDir_IgnoresSubdirectories(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "crd.yaml"), []byte(singleCRDYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "nested.yaml"), []byte(singleCRDYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	crds, err := ParseCRDsFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 1 {
		t.Fatalf("expected 1 CRD (subdirectory ignored), got %d", len(crds))
	}
}
