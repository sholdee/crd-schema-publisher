package extractor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sholdee/crd-schema-publisher/index"
	"github.com/sholdee/crd-schema-publisher/renderer"
)

var (
	writeSchemasFunc       = WriteSchemas
	renderAllFunc          = renderer.RenderAll
	generateIndexFunc      = index.Generate
	activateGenerationFunc = activateGeneration
	cleanLegacyRootFunc    = cleanLegacyRoot
	pruneGenerationsFunc   = pruneGenerations
)

const (
	generationsDirName = ".generations"
	currentLinkName    = "current"
)

type SiteBuildStatus string

const (
	BuildResultNoop  SiteBuildStatus = "noop"
	BuildResultBuilt SiteBuildStatus = "built"
)

type SiteBuildOptions struct {
	Lister    CRDLister
	OutputDir string
	BasePath  string
	Render    bool
	Filter    SchemaFilter
}

type SiteBuildResult struct {
	Status      SiteBuildStatus
	CRDCount    int
	SchemaCount int
}

func BuildSite(opts SiteBuildOptions) (SiteBuildResult, error) {
	if err := ValidateOutputDir(opts.OutputDir); err != nil {
		return SiteBuildResult{}, err
	}

	previousGeneration := currentGenerationName(opts.OutputDir)

	crds, err := ListCRDs(opts.Lister)
	if err != nil {
		return SiteBuildResult{}, fmt.Errorf("listing CRDs: %w", err)
	}
	crds = FilterCRDs(crds, opts.Filter)
	if len(crds) == 0 && !opts.Filter.Active() {
		return SiteBuildResult{Status: BuildResultNoop}, nil
	}

	generationDir, generationName, err := makeGenerationDir(opts.OutputDir)
	if err != nil {
		return SiteBuildResult{}, fmt.Errorf("creating generation dir: %w", err)
	}
	keepGeneration := false
	defer func() {
		if !keepGeneration {
			_ = os.RemoveAll(generationDir)
		}
	}()

	count, err := writeSchemasFunc(crds, generationDir)
	if err != nil {
		return SiteBuildResult{}, fmt.Errorf("writing schemas: %w", err)
	}

	if opts.Render {
		if err := renderAllFunc(generationDir, opts.BasePath); err != nil {
			return SiteBuildResult{}, fmt.Errorf("rendering schemas: %w", err)
		}
	}

	if err := generateIndexFunc(generationDir, opts.BasePath); err != nil {
		return SiteBuildResult{}, fmt.Errorf("generating index: %w", err)
	}

	if err := activateGenerationFunc(opts.OutputDir, generationName); err != nil {
		return SiteBuildResult{}, fmt.Errorf("activating generation: %w", err)
	}
	keepGeneration = true
	if err := cleanLegacyRootFunc(opts.OutputDir); err != nil {
		return SiteBuildResult{}, fmt.Errorf("cleaning legacy root: %w", err)
	}
	if err := pruneGenerationsFunc(opts.OutputDir, generationName, previousGeneration); err != nil {
		return SiteBuildResult{}, fmt.Errorf("pruning generations: %w", err)
	}

	return SiteBuildResult{
		Status:      BuildResultBuilt,
		CRDCount:    len(crds),
		SchemaCount: count,
	}, nil
}

func ValidateOutputDir(outputDir string) error {
	if strings.TrimSpace(outputDir) == "" {
		return fmt.Errorf("OUTPUT_DIR must not be empty")
	}

	clean := filepath.Clean(outputDir)
	if clean == "." || clean == ".." {
		return fmt.Errorf("OUTPUT_DIR %q is unsafe", outputDir)
	}

	abs, err := filepath.Abs(clean)
	if err != nil {
		return fmt.Errorf("resolving OUTPUT_DIR: %w", err)
	}
	if isFilesystemRoot(abs) {
		return fmt.Errorf("OUTPUT_DIR %q must not be filesystem root", outputDir)
	}

	cwd, err := os.Getwd()
	if err == nil {
		if samePath(abs, cwd) {
			return fmt.Errorf("OUTPUT_DIR %q must not be the current working directory", outputDir)
		}
	}

	resolved, err := resolvePath(abs)
	if err == nil {
		if isFilesystemRoot(resolved) {
			return fmt.Errorf("OUTPUT_DIR %q resolves to filesystem root", outputDir)
		}
		if err == nil && cwd != "" {
			if resolvedCWD, cwdErr := resolvePath(cwd); cwdErr == nil && samePath(resolved, resolvedCWD) {
				return fmt.Errorf("OUTPUT_DIR %q resolves to the current working directory", outputDir)
			}
		}
	}

	return nil
}

func makeGenerationDir(outputDir string) (string, string, error) {
	generationsDir := filepath.Join(outputDir, generationsDirName)
	if err := os.MkdirAll(generationsDir, 0o755); err != nil {
		return "", "", err
	}
	prefix := time.Now().UTC().Format("20060102T150405.000000000Z") + "-"
	generationDir, err := os.MkdirTemp(generationsDir, prefix)
	if err != nil {
		return "", "", err
	}
	if err := os.Chmod(generationDir, 0o755); err != nil {
		_ = os.RemoveAll(generationDir)
		return "", "", err
	}
	return generationDir, filepath.Base(generationDir), nil
}

func activateGeneration(outputDir, generationName string) error {
	currentPath := filepath.Join(outputDir, currentLinkName)
	tmpPath := filepath.Join(outputDir, "."+currentLinkName+".tmp")
	target := filepath.Join(generationsDirName, generationName)

	_ = os.Remove(tmpPath)
	if err := os.Symlink(target, tmpPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, currentPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func cleanLegacyRoot(outputDir string) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == generationsDirName || name == currentLinkName {
			continue
		}
		if err := os.RemoveAll(filepath.Join(outputDir, name)); err != nil {
			return err
		}
	}
	return nil
}

func currentGenerationName(outputDir string) string {
	target, err := os.Readlink(filepath.Join(outputDir, currentLinkName))
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

func pruneGenerations(outputDir string, keep ...string) error {
	generationsDir := filepath.Join(outputDir, generationsDirName)
	entries, err := os.ReadDir(generationsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	keepSet := make(map[string]struct{}, len(keep))
	for _, name := range keep {
		if name == "" {
			continue
		}
		keepSet[name] = struct{}{}
	}

	for _, entry := range entries {
		if _, ok := keepSet[entry.Name()]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(generationsDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func resolvePath(path string) (string, error) {
	clean := filepath.Clean(path)
	if isFilesystemRoot(clean) {
		return clean, nil
	}

	var missing []string
	current := clean
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		missing = append([]string{filepath.Base(current)}, missing...)
		current = parent
	}

	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}

	parts := append([]string{resolved}, missing...)
	return filepath.Join(parts...), nil
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func isFilesystemRoot(path string) bool {
	clean := filepath.Clean(path)
	return clean == string(filepath.Separator)
}

func ActiveOutputDir(outputDir string) string {
	return filepath.Join(outputDir, currentLinkName)
}
