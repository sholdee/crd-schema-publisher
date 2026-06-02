package main

import (
	"strings"
	"testing"
)

func mapEnv(values map[string]string) envGetter {
	return func(key string) string {
		return values[key]
	}
}

func TestRuntimeConfig_RunOutputDirFlagOverridesEnv(t *testing.T) {
	cfg, err := parseRuntimeConfig("run", []string{"--output-dir", "/flag-output"}, "/env-output", mapEnv(nil))
	if err != nil {
		t.Fatalf("parseRuntimeConfig: %v", err)
	}
	if cfg.OutputDir != "/flag-output" {
		t.Fatalf("expected flag output dir, got %q", cfg.OutputDir)
	}
}

func TestRuntimeConfig_RunFilterFlagsOverrideEnv(t *testing.T) {
	env := mapEnv(map[string]string{
		schemaFilterKindEnv:    "EnvKind",
		schemaFilterGroupEnv:   "env.example.io",
		schemaFilterVersionEnv: "v1alpha1",
	})
	cfg, err := parseRuntimeConfig("run", []string{"--kind", "FlagKind", "--group", "flag.example.io", "--version", "v1"}, "/output", env)
	if err != nil {
		t.Fatalf("parseRuntimeConfig: %v", err)
	}
	if !cfg.Filter.Matches("FlagKind", "flag.example.io", "v1") {
		t.Fatalf("expected filter to match flag values: %#v", cfg.Filter)
	}
	if cfg.Filter.Matches("EnvKind", "env.example.io", "v1alpha1") {
		t.Fatalf("expected env filter to be overridden by flags: %#v", cfg.Filter)
	}
}

func TestRuntimeConfig_RunIncludeFlagsOverrideEnv(t *testing.T) {
	env := mapEnv(map[string]string{
		schemaIncludeBuiltinsEnv:  "true",
		schemaIncludeKustomizeEnv: "true",
	})
	cfg, err := parseRuntimeConfig("run", []string{"--include-builtins=false", "--include-kustomize=false"}, "/output", env)
	if err != nil {
		t.Fatalf("parseRuntimeConfig: %v", err)
	}
	if cfg.IncludeBuiltins {
		t.Fatal("expected --include-builtins=false to override env")
	}
	if cfg.IncludeKustomize {
		t.Fatal("expected --include-kustomize=false to override env")
	}
}

func TestRuntimeConfig_RunIncludeEnvDefaults(t *testing.T) {
	env := mapEnv(map[string]string{
		schemaIncludeBuiltinsEnv:  "true",
		schemaIncludeKustomizeEnv: "true",
	})
	cfg, err := parseRuntimeConfig("run", nil, "/output", env)
	if err != nil {
		t.Fatalf("parseRuntimeConfig: %v", err)
	}
	if !cfg.IncludeBuiltins {
		t.Fatal("expected SCHEMA_INCLUDE_BUILTINS=true to enable built-ins")
	}
	if !cfg.IncludeKustomize {
		t.Fatal("expected SCHEMA_INCLUDE_KUSTOMIZE=true to enable kustomize")
	}
}

func TestExtractConfig_UsesEnvDefaultsAndFlagOverrides(t *testing.T) {
	env := mapEnv(map[string]string{
		"OUTPUT_DIR":           "/env-output",
		"BASE_PATH":            "/env-base",
		"KUBECTL_CONTEXT":      "env-context",
		"SKIP_RENDER":          "true",
		schemaFilterKindEnv:    "EnvKind",
		schemaFilterGroupEnv:   "env.example.io",
		schemaFilterVersionEnv: "v1alpha1",
	})
	cfg, err := parseExtractConfig([]string{
		"--output-dir", "/flag-output",
		"--base-path", "/flag-base",
		"--context", "flag-context",
		"--skip-render=false",
		"--kind", "FlagKind",
	}, env)
	if err != nil {
		t.Fatalf("parseExtractConfig: %v", err)
	}
	if cfg.OutputDir != "/flag-output" || cfg.BasePath != "/flag-base" || cfg.KubeContext != "flag-context" || !cfg.Render {
		t.Fatalf("unexpected extract config: %#v", cfg)
	}
	if !cfg.Filter.Matches("FlagKind", "env.example.io", "v1alpha1") {
		t.Fatalf("expected mixed flag/env filter values: %#v", cfg.Filter)
	}
}

func TestExtractConfig_IncludeFlagsOverrideEnv(t *testing.T) {
	env := mapEnv(map[string]string{
		"OUTPUT_DIR":              "/env-output",
		schemaIncludeBuiltinsEnv:  "true",
		schemaIncludeKustomizeEnv: "true",
	})
	cfg, err := parseExtractConfig([]string{"--include-builtins=false", "--include-kustomize=false"}, env)
	if err != nil {
		t.Fatalf("parseExtractConfig: %v", err)
	}
	if cfg.IncludeBuiltins {
		t.Fatal("expected --include-builtins=false to override env")
	}
	if cfg.IncludeKustomize {
		t.Fatal("expected --include-kustomize=false to override env")
	}
}

func TestExtractConfig_IncludeEnvDefaults(t *testing.T) {
	env := mapEnv(map[string]string{
		"OUTPUT_DIR":              "/env-output",
		schemaIncludeBuiltinsEnv:  "true",
		schemaIncludeKustomizeEnv: "true",
	})
	cfg, err := parseExtractConfig(nil, env)
	if err != nil {
		t.Fatalf("parseExtractConfig: %v", err)
	}
	if !cfg.IncludeBuiltins {
		t.Fatal("expected SCHEMA_INCLUDE_BUILTINS=true to enable built-ins")
	}
	if !cfg.IncludeKustomize {
		t.Fatal("expected SCHEMA_INCLUDE_KUSTOMIZE=true to enable kustomize")
	}
}

func TestUploadConfig_FlagOutputDirOverridesEnv(t *testing.T) {
	cfg, err := parseUploadCommandConfig([]string{"--output-dir", "/flag-output"}, mapEnv(map[string]string{"OUTPUT_DIR": "/env-output"}))
	if err != nil {
		t.Fatalf("parseUploadCommandConfig: %v", err)
	}
	if cfg.OutputDir != "/flag-output" || !cfg.ExplicitOutputDir {
		t.Fatalf("unexpected upload config: %#v", cfg)
	}
}

func TestPreviewConfig_IgnoresAmbientOutputDir(t *testing.T) {
	cfg, err := parsePreviewConfig(nil)
	if err != nil {
		t.Fatalf("parsePreviewConfig: %v", err)
	}
	if cfg.OutputDir != "" || cfg.ExplicitOutputDir {
		t.Fatalf("preview must ignore ambient OUTPUT_DIR unless a flag is passed: %#v", cfg)
	}
}

func TestCommandConfig_RejectsUnexpectedArgs(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{name: "run", run: func() error {
			_, err := parseRuntimeConfig("run", []string{"unexpected"}, "/output", mapEnv(nil))
			return err
		}, want: "unexpected arguments for run: unexpected"},
		{name: "extract", run: func() error { _, err := parseExtractConfig([]string{"unexpected"}, mapEnv(nil)); return err }, want: "unexpected arguments for extract: unexpected"},
		{name: "upload", run: func() error { _, err := parseUploadCommandConfig([]string{"unexpected"}, mapEnv(nil)); return err }, want: "unexpected arguments for upload: unexpected"},
		{name: "preview", run: func() error { _, err := parsePreviewConfig([]string{"unexpected"}); return err }, want: "unexpected arguments for preview: unexpected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}
}
