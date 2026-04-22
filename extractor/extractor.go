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
	metadataDirName   = "_meta"
	kindsManifestName = "kinds.json"
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

func ListCRDs(lister CRDLister) ([]apiextensionsv1.CustomResourceDefinition, error) {
	list, err := lister.List(context.Background(), metav1.ListOptions{})
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
				kinds[filepath.ToSlash(filepath.Join(group, filename))] = originalKind
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

func writeKindsManifest(outputDir string, kinds map[string]string) error {
	metaDir := filepath.Join(outputDir, metadataDirName)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(kinds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling kinds manifest: %w", err)
	}
	return os.WriteFile(filepath.Join(metaDir, kindsManifestName), data, 0o644)
}
