package extractor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	yamlv3 "gopkg.in/yaml.v3"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	sigsyaml "sigs.k8s.io/yaml"
)

const (
	// maxInputSize is the maximum total YAML input size (256 MB).
	maxInputSize = 256 << 20
	// maxDocuments is the maximum number of YAML documents in a single stream.
	maxDocuments = 10_000
)

// countingReader wraps an io.Reader and tracks the total bytes read.
type countingReader struct {
	r     io.Reader
	count int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.count += int64(n)
	if cr.count > maxInputSize {
		return n, fmt.Errorf("input exceeds maximum size of %d bytes", maxInputSize)
	}
	return n, err
}

// ParseCRDsFromReader decodes a multi-document YAML stream and returns all
// CustomResourceDefinition documents. It also expands CustomResourceDefinitionList
// and generic Kubernetes List documents such as kubectl get crds -o yaml.
// Non-CRD documents are silently skipped. Malformed YAML causes an immediate
// error. Input is capped at 256 MB and 10,000 documents. Uses a streaming YAML
// decoder — documents are parsed one at a time without loading the entire input
// into memory.
func ParseCRDsFromReader(r io.Reader) ([]apiextensionsv1.CustomResourceDefinition, error) {
	cr := &countingReader{r: r}
	dec := yamlv3.NewDecoder(cr)

	var crds []apiextensionsv1.CustomResourceDefinition
	docNum := 0

	for {
		var node yamlv3.Node
		if err := dec.Decode(&node); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("parsing document %d: %w", docNum+1, err)
		}
		docNum++

		if docNum > maxDocuments {
			return nil, fmt.Errorf("input contains more than %d documents", maxDocuments)
		}

		docCRDs, err := parseCRDsFromDocumentNode(&node, docNum)
		if err != nil {
			return nil, err
		}
		crds = append(crds, docCRDs...)
	}

	return crds, nil
}

func parseCRDsFromDocumentNode(node *yamlv3.Node, docNum int) ([]apiextensionsv1.CustomResourceDefinition, error) {
	switch nodeKind(node) {
	case "CustomResourceDefinition":
		crd, err := parseCRDNode(node, docNum)
		if err != nil {
			return nil, err
		}
		return []apiextensionsv1.CustomResourceDefinition{crd}, nil
	case "CustomResourceDefinitionList":
		return parseCRDListNode(node, docNum)
	case "List":
		return parseCRDsFromGenericListNode(node, docNum)
	default:
		return nil, nil
	}
}

func parseCRDNode(node *yamlv3.Node, docNum int) (apiextensionsv1.CustomResourceDefinition, error) {
	raw, err := yamlv3.Marshal(node)
	if err != nil {
		return apiextensionsv1.CustomResourceDefinition{}, fmt.Errorf("re-encoding document %d: %w", docNum, err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := sigsyaml.Unmarshal(raw, &crd); err != nil {
		return apiextensionsv1.CustomResourceDefinition{}, fmt.Errorf("parsing CRD in document %d: %w", docNum, err)
	}
	return crd, nil
}

func parseCRDListNode(node *yamlv3.Node, docNum int) ([]apiextensionsv1.CustomResourceDefinition, error) {
	raw, err := yamlv3.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("re-encoding document %d: %w", docNum, err)
	}
	var list apiextensionsv1.CustomResourceDefinitionList
	if err := sigsyaml.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("parsing CRD list in document %d: %w", docNum, err)
	}
	return list.Items, nil
}

func parseCRDsFromGenericListNode(node *yamlv3.Node, docNum int) ([]apiextensionsv1.CustomResourceDefinition, error) {
	if node.Kind == yamlv3.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	if node.Kind != yamlv3.MappingNode {
		return nil, nil
	}

	var items *yamlv3.Node
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == "items" {
			items = node.Content[i+1]
			break
		}
	}
	if items == nil {
		return nil, nil
	}
	if items.Kind != yamlv3.SequenceNode {
		return nil, fmt.Errorf("parsing Kubernetes list in document %d: items must be a sequence", docNum)
	}

	var crds []apiextensionsv1.CustomResourceDefinition
	for i, item := range items.Content {
		if nodeKind(item) != "CustomResourceDefinition" {
			continue
		}
		crd, err := parseCRDNode(item, docNum)
		if err != nil {
			return nil, withCRDItemContext(err, i+1, docNum)
		}
		crds = append(crds, crd)
	}
	return crds, nil
}

func withCRDItemContext(err error, itemNum, docNum int) error {
	return fmt.Errorf("parsing CRD item %d in document %d: %w", itemNum, docNum, err)
}

// nodeKind returns the top-level Kubernetes kind without fully deserializing
// the document.
func nodeKind(node *yamlv3.Node) string {
	// Document nodes wrap the actual content
	if node.Kind == yamlv3.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	if node.Kind != yamlv3.MappingNode {
		return ""
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == "kind" {
			return node.Content[i+1].Value
		}
	}
	return ""
}

func ParseCRDsFromFiles(paths []string) ([]apiextensionsv1.CustomResourceDefinition, error) {
	var all []apiextensionsv1.CustomResourceDefinition
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", path, err)
		}
		crds, err := ParseCRDsFromReader(f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		all = append(all, crds...)
	}
	return all, nil
}

func ParseCRDsFromDir(dir string) ([]apiextensionsv1.CustomResourceDefinition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".yaml" || ext == ".yml" {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}

	return ParseCRDsFromFiles(paths)
}
