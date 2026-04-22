package extractor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sholdee/crd-schema-publisher/index"
	"github.com/sholdee/crd-schema-publisher/renderer"
)

var (
	writeSchemasFunc = WriteSchemas
	renderAllFunc    = renderer.RenderAll
	generateIndexFunc = index.Generate
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

	crds, err := ListCRDs(opts.Lister)
	if err != nil {
		return SiteBuildResult{}, fmt.Errorf("listing CRDs: %w", err)
	}
	if len(crds) == 0 {
		return SiteBuildResult{Status: BuildResultNoop}, nil
	}

	stagingDir, err := makeStagingDir(opts.OutputDir)
	if err != nil {
		return SiteBuildResult{}, fmt.Errorf("creating staging dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(stagingDir)
	}()

	count, err := writeSchemasFunc(crds, stagingDir)
	if err != nil {
		return SiteBuildResult{}, fmt.Errorf("writing schemas: %w", err)
	}

	if opts.Render {
		if err := renderAllFunc(stagingDir, opts.BasePath); err != nil {
			return SiteBuildResult{}, fmt.Errorf("rendering schemas: %w", err)
		}
	}

	if err := generateIndexFunc(stagingDir, opts.BasePath); err != nil {
		return SiteBuildResult{}, fmt.Errorf("generating index: %w", err)
	}

	if err := promoteSite(stagingDir, opts.OutputDir); err != nil {
		return SiteBuildResult{}, fmt.Errorf("promoting output: %w", err)
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

func makeStagingDir(outputDir string) (string, error) {
	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	return os.MkdirTemp(parent, "."+filepath.Base(outputDir)+".staging-*")
}

func promoteSite(stagingDir, outputDir string) error {
	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	backupDir, err := os.MkdirTemp(parent, "."+filepath.Base(outputDir)+".backup-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(backupDir)
	}()

	currentEntries, err := os.ReadDir(outputDir)
	if err != nil {
		return err
	}
	movedCurrent := make([]string, 0, len(currentEntries))
	for _, entry := range currentEntries {
		name := entry.Name()
		if err := os.Rename(filepath.Join(outputDir, name), filepath.Join(backupDir, name)); err != nil {
			for i := len(movedCurrent) - 1; i >= 0; i-- {
				rollbackName := movedCurrent[i]
				_ = os.Rename(filepath.Join(backupDir, rollbackName), filepath.Join(outputDir, rollbackName))
			}
			return err
		}
		movedCurrent = append(movedCurrent, name)
	}

	stagedEntries, err := os.ReadDir(stagingDir)
	if err != nil {
		for i := len(movedCurrent) - 1; i >= 0; i-- {
			rollbackName := movedCurrent[i]
			_ = os.Rename(filepath.Join(backupDir, rollbackName), filepath.Join(outputDir, rollbackName))
		}
		return err
	}
	movedStaged := make([]string, 0, len(stagedEntries))
	for _, entry := range stagedEntries {
		name := entry.Name()
		if err := os.Rename(filepath.Join(stagingDir, name), filepath.Join(outputDir, name)); err != nil {
			for i := len(movedStaged) - 1; i >= 0; i-- {
				rollbackName := movedStaged[i]
				_ = os.Rename(filepath.Join(outputDir, rollbackName), filepath.Join(stagingDir, rollbackName))
			}
			for i := len(movedCurrent) - 1; i >= 0; i-- {
				rollbackName := movedCurrent[i]
				_ = os.Rename(filepath.Join(backupDir, rollbackName), filepath.Join(outputDir, rollbackName))
			}
			return err
		}
		movedStaged = append(movedStaged, name)
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
