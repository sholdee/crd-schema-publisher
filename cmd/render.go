package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/sholdee/crd-schema-publisher/extractor"
	"github.com/sholdee/crd-schema-publisher/index"
	"github.com/sholdee/crd-schema-publisher/renderer"
)

func init() {
	registerCommand("render", runRender)
}

func runRender(args []string) error {
	cfg, err := parseRenderConfig(args, os.Getenv)
	if err != nil {
		return err
	}
	if err := requireExistingOutputDir(cfg.OutputDir, "Set OUTPUT_DIR or pass --output-dir to a directory containing extracted or converted schemas"); err != nil {
		return err
	}

	renderDir, err := resolveRenderDir(cfg.OutputDir)
	if err != nil {
		return err
	}

	manifest, err := readConvertManifest(renderDir)
	if err != nil {
		return err
	}
	var preRender map[string]bool
	if manifest != nil {
		preRender = snapshotFiles(renderDir)
	}

	slog.Info("rendering schema pages", "dir", renderDir)
	if err := renderOutput(renderDir, cfg.BasePath); err != nil {
		return err
	}

	if manifest != nil {
		// Record rendered files in the convert manifest so a later convert
		// run cleans them instead of leaving stale pages behind. Treating
		// manifest entries as non-pre-existing keeps them recorded alongside
		// the new render output.
		for _, rel := range manifest {
			delete(preRender, rel)
		}
		if err := writeConvertManifest(renderDir, preRender); err != nil {
			return fmt.Errorf("updating convert manifest: %w", err)
		}
	}

	slog.Info("render complete", "dir", renderDir)
	return nil
}

// resolveRenderDir returns the active generation when outputDir uses the
// runtime layout (a "current" symlink into .generations); otherwise the
// directory itself is rendered, matching the flat convert layout.
func resolveRenderDir(outputDir string) (string, error) {
	activeDir := extractor.ActiveOutputDir(outputDir)
	resolved, err := filepath.EvalSymlinks(activeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return outputDir, nil
		}
		return "", fmt.Errorf("resolving active output: %w", err)
	}
	return resolved, nil
}

func renderOutput(outputDir, basePath string) error {
	normalizedBasePath := normalizeBasePath(basePath)
	if err := renderer.RenderAll(outputDir, normalizedBasePath); err != nil {
		return fmt.Errorf("rendering schemas: %w", err)
	}
	if err := index.Generate(outputDir, normalizedBasePath); err != nil {
		return fmt.Errorf("generating index: %w", err)
	}
	return nil
}
