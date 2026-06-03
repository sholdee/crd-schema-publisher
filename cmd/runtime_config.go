package main

import (
	"fmt"
	"strings"

	"github.com/sholdee/crd-schema-publisher/extractor"
)

type envGetter func(string) string

type runtimeConfig struct {
	OutputDir        string
	Filter           extractor.SchemaFilter
	IncludeBuiltins  bool
	IncludeKustomize bool
}

type runtimeCommandOptions = runtimeConfig

type extractConfig struct {
	OutputDir        string
	BasePath         string
	KubeContext      string
	Render           bool
	Filter           extractor.SchemaFilter
	IncludeBuiltins  bool
	IncludeKustomize bool
}

type uploadCommandConfig struct {
	OutputDir         string
	ExplicitOutputDir bool
}

type previewConfig struct {
	OutputDir         string
	ExplicitOutputDir bool
}

func envDefault(env envGetter, key, fallback string) string {
	if env == nil {
		return fallback
	}
	if v := env(key); v != "" {
		return v
	}
	return fallback
}

func parseRuntimeConfig(cmd string, args []string, fallbackOutputDir string, env envGetter) (runtimeConfig, error) {
	fs := newCommandFlagSet(cmd)
	var outputDir string
	stringFlagWithAlias(fs, &outputDir, "output-dir", "o", fallbackOutputDir, "output directory")
	kind := fs.String("kind", envDefault(env, schemaFilterKindEnv, ""), "filter by kind (comma-separated, case-insensitive)")
	group := fs.String("group", envDefault(env, schemaFilterGroupEnv, ""), "filter by group (comma-separated, case-insensitive)")
	version := fs.String("version", envDefault(env, schemaFilterVersionEnv, ""), "filter by version (comma-separated, case-insensitive)")
	includeBuiltins := fs.Bool("include-builtins", envDefault(env, schemaIncludeBuiltinsEnv, "") == "true", "include Kubernetes built-in schemas from the API server")
	includeKustomize := fs.Bool("include-kustomize", envDefault(env, schemaIncludeKustomizeEnv, "") == "true", "include kustomize config schemas")
	if err := fs.Parse(args); err != nil {
		return runtimeConfig{}, err
	}
	if extras := fs.Args(); len(extras) > 0 {
		return runtimeConfig{}, fmt.Errorf("unexpected arguments for %s: %s", cmd, strings.Join(extras, " "))
	}
	return runtimeConfig{
		OutputDir:        outputDir,
		Filter:           extractor.ParseFilter(*kind, *group, *version),
		IncludeBuiltins:  *includeBuiltins,
		IncludeKustomize: *includeKustomize,
	}, nil
}

func parseExtractConfig(args []string, env envGetter) (extractConfig, error) {
	fs := newCommandFlagSet("extract")
	var outputDir string
	stringFlagWithAlias(fs, &outputDir, "output-dir", "o", envDefault(env, "OUTPUT_DIR", ""), "output directory")
	basePath := fs.String("base-path", envDefault(env, "BASE_PATH", ""), "URL path prefix for subpath deployments")
	kubeContext := fs.String("context", envDefault(env, "KUBECTL_CONTEXT", ""), "Kubernetes context")
	skipRender := fs.Bool("skip-render", envDefault(env, "SKIP_RENDER", "") == "true", "skip HTML rendering")
	kind := fs.String("kind", envDefault(env, schemaFilterKindEnv, ""), "filter by kind (comma-separated, case-insensitive)")
	group := fs.String("group", envDefault(env, schemaFilterGroupEnv, ""), "filter by group (comma-separated, case-insensitive)")
	version := fs.String("version", envDefault(env, schemaFilterVersionEnv, ""), "filter by version (comma-separated, case-insensitive)")
	includeBuiltins := fs.Bool("include-builtins", envDefault(env, schemaIncludeBuiltinsEnv, "") == "true", "include Kubernetes built-in schemas from the API server")
	includeKustomize := fs.Bool("include-kustomize", envDefault(env, schemaIncludeKustomizeEnv, "") == "true", "include kustomize config schemas")
	if err := fs.Parse(args); err != nil {
		return extractConfig{}, err
	}
	if extras := fs.Args(); len(extras) > 0 {
		return extractConfig{}, fmt.Errorf("unexpected arguments for extract: %s", strings.Join(extras, " "))
	}
	return extractConfig{
		OutputDir:        outputDir,
		BasePath:         *basePath,
		KubeContext:      *kubeContext,
		Render:           !*skipRender,
		Filter:           extractor.ParseFilter(*kind, *group, *version),
		IncludeBuiltins:  *includeBuiltins,
		IncludeKustomize: *includeKustomize,
	}, nil
}

func parseUploadCommandConfig(args []string, env envGetter) (uploadCommandConfig, error) {
	outputDir, explicit, err := parseOutputDirArg("upload", args, envDefault(env, "OUTPUT_DIR", "/output"))
	if err != nil {
		return uploadCommandConfig{}, err
	}
	return uploadCommandConfig{OutputDir: outputDir, ExplicitOutputDir: explicit}, nil
}

func parsePreviewConfig(args []string) (previewConfig, error) {
	outputDir, explicit, err := parseOutputDirArg("preview", args, "")
	if err != nil {
		return previewConfig{}, err
	}
	return previewConfig{OutputDir: outputDir, ExplicitOutputDir: explicit}, nil
}
