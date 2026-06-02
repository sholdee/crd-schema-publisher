package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sholdee/crd-schema-publisher/converter"
	"github.com/sholdee/crd-schema-publisher/schemametadata"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// CRDLister abstracts the Kubernetes CRD list operation for testability.
type CRDLister interface {
	List(ctx context.Context, opts metav1.ListOptions) (*apiextensionsv1.CustomResourceDefinitionList, error)
}

const (
	metadataDirName   = schemametadata.MetadataDirName
	kindsManifestName = schemametadata.KindsManifestName
)

func BuildConfig(kubeContext string) (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		overrides := &clientcmd.ConfigOverrides{}
		if kubeContext != "" {
			overrides.CurrentContext = kubeContext
		}
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("building kubeconfig: %w", err)
		}
	}
	return cfg, nil
}

func BuildClient(kubeContext string) (*apiextensionsclient.Clientset, error) {
	cfg, err := BuildConfig(kubeContext)
	if err != nil {
		return nil, err
	}
	return apiextensionsclient.NewForConfig(cfg)
}

func ListCRDs(ctx context.Context, lister CRDLister) ([]apiextensionsv1.CustomResourceDefinition, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	list, err := lister.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing CRDs: %w", err)
	}
	return list.Items, nil
}

func WriteSchemas(crds []apiextensionsv1.CustomResourceDefinition, outputDir string) (int, error) {
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		sem      = make(chan struct{}, 10)
		count    int
		firstErr error
		kinds    = make(map[string]string)
		metadata = make(map[string]schemametadata.SchemaMetadataEntry)
	)

	for _, crd := range crds {
		for _, version := range crd.Spec.Versions {
			var schemaProps *apiextensionsv1.JSONSchemaProps
			if version.Schema != nil && version.Schema.OpenAPIV3Schema != nil {
				schemaProps = version.Schema.OpenAPIV3Schema
			}
			if schemaProps == nil {
				continue
			}

			kind := strings.ToLower(crd.Spec.Names.Kind)
			group := crd.Spec.Group
			versionName := version.Name

			wg.Add(1)
			sem <- struct{}{}
			go func(props *apiextensionsv1.JSONSchemaProps, kind, group, versionName, originalKind string) {
				defer wg.Done()
				defer func() { <-sem }()

				filename, err := writeSchemaFiles(props, kind, group, versionName, outputDir)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}

				mu.Lock()
				relPath := filepath.ToSlash(filepath.Join(group, filename))
				kinds[relPath] = originalKind
				metadata[relPath] = schemametadata.SchemaMetadataEntry{Kind: originalKind, Source: schemametadata.SchemaSourceCRD}
				count++
				mu.Unlock()
			}(schemaProps, kind, group, versionName, crd.Spec.Names.Kind)
		}
	}

	wg.Wait()
	if firstErr != nil {
		return count, firstErr
	}
	if len(kinds) == 0 {
		return count, nil
	}
	if err := writeKindsManifest(outputDir, kinds); err != nil {
		return count, err
	}
	if err := writeSchemaMetadataManifest(outputDir, metadata); err != nil {
		return count, err
	}
	return count, firstErr
}

func writeSchemaFiles(props *apiextensionsv1.JSONSchemaProps, kind, group, versionName, outputDir string) (string, error) {
	raw, err := json.Marshal(props)
	if err != nil {
		return "", fmt.Errorf("marshaling schema for %s/%s: %w", group, kind, err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return "", fmt.Errorf("unmarshaling schema for %s/%s: %w", group, kind, err)
	}

	return writeSchemaMap(schema, kind, group, versionName, outputDir)
}

// writeSchemaMap runs the converter over an already-decoded schema and writes
// the grouped and master-standalone files. Shared by CRD and OpenAPI inputs.
func writeSchemaMap(schema map[string]interface{}, kind, group, versionName, outputDir string) (string, error) {
	schema = converter.Convert(schema)

	jsonBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling JSON for %s/%s: %w", group, kind, err)
	}

	groupDir := filepath.Join(outputDir, group)
	if err := os.MkdirAll(groupDir, 0o755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%s_%s.json", kind, versionName)
	if err := os.WriteFile(filepath.Join(groupDir, filename), jsonBytes, 0o644); err != nil {
		return "", err
	}

	standaloneDir := filepath.Join(outputDir, "master-standalone")
	if err := os.MkdirAll(standaloneDir, 0o755); err != nil {
		return "", err
	}
	standaloneName := fmt.Sprintf("%s-%s-stable-%s.json", group, kind, versionName)
	if err := os.WriteFile(filepath.Join(standaloneDir, standaloneName), jsonBytes, 0o644); err != nil {
		return "", err
	}
	return filename, nil
}

// writeKindsManifest merges kinds into any existing manifest, so multiple
// schema writer passes into the same directory share one index.
func writeKindsManifest(outputDir string, kinds map[string]string) error {
	metaDir := filepath.Join(outputDir, metadataDirName)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}

	manifestPath := filepath.Join(metaDir, kindsManifestName)
	if existing, err := os.ReadFile(manifestPath); err == nil {
		var prev map[string]string
		if err := json.Unmarshal(existing, &prev); err == nil {
			for path, kind := range kinds {
				prev[path] = kind
			}
			kinds = prev
		}
	}

	data, err := json.MarshalIndent(kinds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling kinds manifest: %w", err)
	}
	return os.WriteFile(manifestPath, data, 0o644)
}

// writeSchemaMetadataManifest mirrors writeKindsManifest for source metadata.
func writeSchemaMetadataManifest(outputDir string, entries map[string]schemametadata.SchemaMetadataEntry) error {
	metaDir := filepath.Join(outputDir, metadataDirName)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}

	manifestPath := filepath.Join(metaDir, schemametadata.SchemaMetadataManifestName)
	if existing, err := os.ReadFile(manifestPath); err == nil {
		var prev map[string]schemametadata.SchemaMetadataEntry
		if err := json.Unmarshal(existing, &prev); err == nil {
			for path, entry := range entries {
				prev[path] = entry
			}
			entries = prev
		}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling schema metadata manifest: %w", err)
	}
	return os.WriteFile(manifestPath, data, 0o644)
}
