package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/index"
	"github.com/sholdee/crd-schema-publisher/renderer"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func init() {
	registerCommand("convert", runConvert)
}

func parseCRDsFromFileList(fileFlag string) ([]apiextensionsv1.CustomResourceDefinition, error) {
	var crds []apiextensionsv1.CustomResourceDefinition
	for _, f := range strings.Split(fileFlag, ",") {
		f = strings.TrimSpace(f)
		if f == "-" {
			stdinCRDs, err := extractor.ParseCRDsFromReader(os.Stdin)
			if err != nil {
				return nil, fmt.Errorf("parsing stdin: %w", err)
			}
			crds = append(crds, stdinCRDs...)
		} else {
			fileCRDs, err := extractor.ParseCRDsFromFiles([]string{f})
			if err != nil {
				return nil, err
			}
			crds = append(crds, fileCRDs...)
		}
	}
	return crds, nil
}

const convertManifestPath = "_meta/convert-manifest.json"

// cleanConvertArtifacts reads the convert manifest from a prior run and removes
// exactly those files. After file removal, empty parent directories are pruned
// bottom-up so user files inside generated directories are preserved. Returns
// an error if the manifest exists but is corrupt — the user must resolve this
// manually (delete the output directory or fix the manifest) rather than risk
// mixed stale-and-fresh output.
func cleanConvertArtifacts(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}

	manifest, err := readConvertManifest(dir)
	if err != nil {
		return err
	}
	if manifest == nil {
		return nil // no manifest file = nothing to clean (first run)
	}

	// Remove only the individual files listed in the manifest
	parentDirs := make(map[string]bool)
	for _, rel := range manifest {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		_ = os.Remove(abs)
		// Track all ancestor directories (relative to output dir) for pruning
		for parent := filepath.Dir(rel); parent != "." && parent != ""; parent = filepath.Dir(parent) {
			parentDirs[filepath.Join(dir, filepath.FromSlash(parent))] = true
		}
	}

	// Remove the manifest and _meta directory
	_ = os.Remove(filepath.Join(dir, convertManifestPath))
	parentDirs[filepath.Join(dir, "_meta")] = true

	// Prune empty directories bottom-up — longest paths first so children
	// are removed before parents. os.Remove on a directory only succeeds
	// if it is empty, so user files inside generated directories survive.
	sorted := make([]string, 0, len(parentDirs))
	for d := range parentDirs {
		sorted = append(sorted, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(sorted)))
	for _, d := range sorted {
		_ = os.Remove(d) // only removes if empty
	}

	return nil
}

// readConvertManifest reads the convert manifest. Returns (nil, nil) if the
// manifest file does not exist (first run). Returns a non-nil error if the
// file exists but cannot be parsed — this is a corruption condition.
func readConvertManifest(dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, convertManifestPath))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading convert manifest: %w", err)
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, fmt.Errorf("corrupt convert manifest in %s — delete the output directory or fix the manifest manually: %w",
			filepath.Join(dir, convertManifestPath), err)
	}
	return paths, nil
}

// snapshotFiles returns the set of file paths (relative, slash-separated)
// present in dir before convert writes anything. Used to distinguish
// pre-existing user files from convert output.
func snapshotFiles(dir string) map[string]bool {
	files := make(map[string]bool)
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil //nolint:nilerr // intentionally skip unreadable entries
		}
		if rel, relErr := filepath.Rel(dir, path); relErr == nil {
			files[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	return files
}

// writeConvertManifest walks the output directory and records only files that
// were NOT in the pre-existing snapshot. This ensures user files present before
// convert ran are never recorded as convert output.
func writeConvertManifest(dir string, preExisting map[string]bool) error {
	var paths []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if !preExisting[rel] {
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking output directory: %w", err)
	}

	metaDir := filepath.Join(dir, "_meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, convertManifestPath), append(data, '\n'), 0o644)
}

func loadCRDs(fileList, dir string) ([]apiextensionsv1.CustomResourceDefinition, error) {
	var crds []apiextensionsv1.CustomResourceDefinition
	if fileList != "" {
		fileCRDs, err := parseCRDsFromFileList(fileList)
		if err != nil {
			return nil, err
		}
		crds = append(crds, fileCRDs...)
	}
	if dir != "" {
		dirCRDs, err := extractor.ParseCRDsFromDir(dir)
		if err != nil {
			return nil, err
		}
		crds = append(crds, dirCRDs...)
	}
	return crds, nil
}

func runConvert(args []string) error {
	fs := newCommandFlagSet("convert")
	var fileFlag string
	stringFlagWithAlias(fs, &fileFlag, "file", "f", "", "CRD YAML file(s), comma-separated (use - for stdin)")
	var dirFlag string
	stringFlagWithAlias(fs, &dirFlag, "dir", "d", "", "directory containing CRD YAML files")
	var outputDir string
	stringFlagWithAlias(fs, &outputDir, "output-dir", "o", "", "output directory for JSON schemas (required)")
	basePath := fs.String("base-path", os.Getenv("BASE_PATH"), "URL path prefix for subpath deployments")
	render := fs.Bool("render", false, "render HTML documentation pages")
	kind := fs.String("kind", "", "filter by kind (comma-separated, case-insensitive)")
	group := fs.String("group", "", "filter by group (comma-separated, case-insensitive)")
	version := fs.String("version", "", "filter by version (comma-separated, case-insensitive)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if outputDir == "" {
		return fmt.Errorf("--output-dir is required")
	}
	if err := extractor.ValidateOutputDir(outputDir); err != nil {
		return err
	}
	if fileFlag == "" && dirFlag == "" {
		return fmt.Errorf("at least one of --file or --dir is required")
	}

	crds, err := loadCRDs(fileFlag, dirFlag)
	if err != nil {
		return err
	}

	crds = extractor.FilterCRDs(crds, extractor.ParseFilter(*kind, *group, *version))

	if err := cleanConvertArtifacts(outputDir); err != nil {
		return fmt.Errorf("preparing output directory: %w", err)
	}

	if len(crds) == 0 {
		slog.Info("no CRDs matched filters")
		return nil
	}

	preExisting := snapshotFiles(outputDir)

	count, err := extractor.WriteSchemas(crds, outputDir)
	if err != nil {
		return fmt.Errorf("writing schemas: %w", err)
	}

	normalizedBasePath := normalizeBasePath(*basePath)
	if *render {
		if err := renderer.RenderAll(outputDir, normalizedBasePath); err != nil {
			return fmt.Errorf("rendering schemas: %w", err)
		}
		if err := index.Generate(outputDir, normalizedBasePath); err != nil {
			return fmt.Errorf("generating index: %w", err)
		}
	}

	if err := writeConvertManifest(outputDir, preExisting); err != nil {
		return fmt.Errorf("writing convert manifest: %w", err)
	}

	slog.Info("convert complete", "count", count, "dir", outputDir)
	return nil
}
