package main

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sholdee/crd-schema-publisher/extractor"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/rest"
)

func TestNormalizeBasePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"normal", "/iac", "/iac"},
		{"trailing slash", "/iac/", "/iac"},
		{"no leading slash", "iac", "/iac"},
		{"both issues", "iac/", "/iac"},
		{"just slash", "/", ""},
		{"multi-segment", "/docs/schemas", "/docs/schemas"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeBasePath(tt.input); got != tt.expected {
				t.Errorf("normalizeBasePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestInitLogger_JSONForRun(t *testing.T) {
	orig := slog.Default()
	defer slog.SetDefault(orig)
	initLogger("run")
	handler := slog.Default().Handler()
	if _, ok := handler.(*slog.JSONHandler); !ok {
		t.Errorf("expected JSONHandler for 'run', got %T", handler)
	}
}

func TestInitLogger_JSONForWatch(t *testing.T) {
	orig := slog.Default()
	defer slog.SetDefault(orig)
	initLogger("watch")
	handler := slog.Default().Handler()
	if _, ok := handler.(*slog.JSONHandler); !ok {
		t.Errorf("expected JSONHandler for 'watch', got %T", handler)
	}
}

func TestInitLogger_TextForPreview(t *testing.T) {
	orig := slog.Default()
	defer slog.SetDefault(orig)
	initLogger("preview")
	handler := slog.Default().Handler()
	if _, ok := handler.(*slog.TextHandler); !ok {
		t.Errorf("expected TextHandler for 'preview', got %T", handler)
	}
}

func TestPrintUsage_ContainsAllCommands(t *testing.T) {
	var buf strings.Builder
	printUsage(&buf)
	output := buf.String()
	for _, cmd := range commandSpecs {
		if !strings.Contains(output, cmd.name) {
			t.Errorf("usage output missing command %q", cmd.name)
		}
	}
}

func TestRegisteredCommandsMatchAdvertisedCommands(t *testing.T) {
	advertised := make(map[string]bool)
	for _, cmd := range commandSpecs {
		advertised[cmd.name] = true
		if commands[cmd.name] == nil {
			t.Errorf("advertised command %q is not registered", cmd.name)
		}
	}
	for name := range commands {
		if !advertised[name] {
			t.Errorf("registered command %q is not advertised in usage", name)
		}
	}
}

func TestSingleFileMainSupportsTopLevelHelpAndVersion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"--help"}, want: "crd-schema-publisher"},
		{name: "version", args: []string{"--version"}, want: "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"run", "./main.go"}, tt.args...)
			cmd := exec.Command("go", args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, output)
			}
			if !strings.Contains(string(output), tt.want) {
				t.Fatalf("expected output to contain %q, got:\n%s", tt.want, output)
			}
		})
	}
}

func TestParseSubcommand_DefaultsToRun(t *testing.T) {
	cmd, args := parseSubcommand([]string{"crd-schema-publisher"})
	if cmd != "run" {
		t.Errorf("expected default 'run', got %q", cmd)
	}
	if len(args) != 0 {
		t.Errorf("expected no args, got %v", args)
	}
}

func TestParseSubcommand_LeadingFlagDefaultsToRun(t *testing.T) {
	cmd, args := parseSubcommand([]string{"crd-schema-publisher", "--output-dir", "/tmp/out"})
	if cmd != "run" {
		t.Errorf("expected default 'run' for leading flag, got %q", cmd)
	}
	if len(args) != 2 || args[0] != "--output-dir" || args[1] != "/tmp/out" {
		t.Errorf("expected ['--output-dir', '/tmp/out'], got %v", args)
	}
}

func TestParseSubcommand_LeadingOutputDirEqualsDefaultsToRun(t *testing.T) {
	cmd, args := parseSubcommand([]string{"crd-schema-publisher", "--output-dir=/tmp/out"})
	if cmd != "run" {
		t.Errorf("expected default 'run' for --output-dir=<path>, got %q", cmd)
	}
	if len(args) != 1 || args[0] != "--output-dir=/tmp/out" {
		t.Errorf("expected ['--output-dir=/tmp/out'], got %v", args)
	}
}

func TestParseSubcommand_LeadingFilterFlagDefaultsToRun(t *testing.T) {
	cmd, args := parseSubcommand([]string{"crd-schema-publisher", "--kind", "certificate"})
	if cmd != "run" {
		t.Errorf("expected default 'run' for leading filter flag, got %q", cmd)
	}
	if len(args) != 2 || args[0] != "--kind" || args[1] != "certificate" {
		t.Errorf("expected ['--kind', 'certificate'], got %v", args)
	}
}

func TestParseSubcommand_LeadingVersionFilterFlagDefaultsToRun(t *testing.T) {
	cmd, args := parseSubcommand([]string{"crd-schema-publisher", "--version", "v1"})
	if cmd != "run" {
		t.Errorf("expected default 'run' for leading version filter flag, got %q", cmd)
	}
	if len(args) != 2 || args[0] != "--version" || args[1] != "v1" {
		t.Errorf("expected ['--version', 'v1'], got %v", args)
	}
}

func TestParseSubcommand_NonRunFlagDoesNotDefaultToRun(t *testing.T) {
	cmd, args := parseSubcommand([]string{"crd-schema-publisher", "--base-path", "/docs"})
	if cmd != "--base-path" {
		t.Errorf("expected raw leading flag command '--base-path', got %q", cmd)
	}
	if len(args) != 1 || args[0] != "/docs" {
		t.Errorf("expected ['/docs'], got %v", args)
	}
}

func TestParseSubcommand_ExtractsCommand(t *testing.T) {
	cmd, args := parseSubcommand([]string{"crd-schema-publisher", "extract", "--output-dir", "/tmp"})
	if cmd != "extract" {
		t.Errorf("expected 'extract', got %q", cmd)
	}
	if len(args) != 2 || args[0] != "--output-dir" {
		t.Errorf("expected ['--output-dir', '/tmp'], got %v", args)
	}
}

func TestParseSubcommand_HelpFlag(t *testing.T) {
	cmd, _ := parseSubcommand([]string{"crd-schema-publisher", "--help"})
	if cmd != "help" {
		t.Errorf("expected 'help', got %q", cmd)
	}

	cmd, _ = parseSubcommand([]string{"crd-schema-publisher", "-h"})
	if cmd != "help" {
		t.Errorf("expected 'help' for -h, got %q", cmd)
	}
}

func TestParseSubcommand_VersionFlag(t *testing.T) {
	cmd, _ := parseSubcommand([]string{"crd-schema-publisher", "--version"})
	if cmd != "version" {
		t.Errorf("expected 'version', got %q", cmd)
	}

	cmd, _ = parseSubcommand([]string{"crd-schema-publisher", "-v"})
	if cmd != "version" {
		t.Errorf("expected 'version' for -v, got %q", cmd)
	}
}

func TestVersionDefault(t *testing.T) {
	if version != "dev" {
		t.Errorf("expected default version 'dev', got %q", version)
	}
}

func TestRunExtract_FlagsOverrideEnvVars(t *testing.T) {
	clientOrig := buildClientFunc
	buildOrig := buildSiteFunc
	defer func() {
		buildClientFunc = clientOrig
		buildSiteFunc = buildOrig
	}()

	var capturedOpts extractor.SiteBuildOptions
	var capturedContext string
	buildClientFunc = func(kubeContext string) (*apiextensionsclient.Clientset, error) {
		capturedContext = kubeContext
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}
	buildSiteFunc = func(opts extractor.SiteBuildOptions) (extractor.SiteBuildResult, error) {
		capturedOpts = opts
		return extractor.SiteBuildResult{Status: extractor.BuildResultNoop}, nil
	}

	t.Setenv("OUTPUT_DIR", "/env-output")
	t.Setenv("KUBECTL_CONTEXT", "env-context")

	tmpDir := t.TempDir()
	err := runExtract([]string{"--output-dir", tmpDir, "--context", "flag-context"})
	if err != nil {
		t.Fatalf("runExtract error: %v", err)
	}
	if capturedContext != "flag-context" {
		t.Errorf("expected flag context 'flag-context', got %q", capturedContext)
	}
	if capturedOpts.OutputDir != tmpDir {
		t.Errorf("expected flag output-dir %q, got %q", tmpDir, capturedOpts.OutputDir)
	}
}

func TestRunExtract_FallsBackToEnvVars(t *testing.T) {
	clientOrig := buildClientFunc
	buildOrig := buildSiteFunc
	defer func() {
		buildClientFunc = clientOrig
		buildSiteFunc = buildOrig
	}()

	var capturedOpts extractor.SiteBuildOptions
	buildClientFunc = func(kubeContext string) (*apiextensionsclient.Clientset, error) {
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}
	buildSiteFunc = func(opts extractor.SiteBuildOptions) (extractor.SiteBuildResult, error) {
		capturedOpts = opts
		return extractor.SiteBuildResult{Status: extractor.BuildResultNoop}, nil
	}

	tmpDir := t.TempDir()
	t.Setenv("OUTPUT_DIR", tmpDir)
	t.Setenv("KUBECTL_CONTEXT", "env-context")
	t.Setenv("SCHEMA_FILTER_KIND", "Certificate,Issuer")
	t.Setenv("SCHEMA_FILTER_GROUP", "Cert-Manager.IO")
	t.Setenv("SCHEMA_FILTER_VERSION", "V1")

	err := runExtract([]string{})
	if err != nil {
		t.Fatalf("runExtract error: %v", err)
	}
	if capturedOpts.OutputDir != tmpDir {
		t.Errorf("expected env OUTPUT_DIR %q, got %q", tmpDir, capturedOpts.OutputDir)
	}
	if got := strings.Join(capturedOpts.Filter.Kinds, ","); got != "certificate,issuer" {
		t.Errorf("expected env kind filter, got %q", got)
	}
	if got := strings.Join(capturedOpts.Filter.Groups, ","); got != "cert-manager.io" {
		t.Errorf("expected env group filter, got %q", got)
	}
	if got := strings.Join(capturedOpts.Filter.Versions, ","); got != "v1" {
		t.Errorf("expected env version filter, got %q", got)
	}
}

func TestRunExtract_RequiresExplicitOutputDir(t *testing.T) {
	clientOrig := buildClientFunc
	defer func() {
		buildClientFunc = clientOrig
	}()

	called := false
	buildClientFunc = func(kubeContext string) (*apiextensionsclient.Clientset, error) {
		called = true
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}

	t.Setenv("OUTPUT_DIR", "")
	t.Setenv("KUBECTL_CONTEXT", "")

	err := runExtract(nil)
	if err == nil {
		t.Fatal("expected runExtract error")
	}
	if !strings.Contains(err.Error(), "OUTPUT_DIR is required") {
		t.Fatalf("expected required output dir error, got %v", err)
	}
	if !strings.Contains(err.Error(), "pass --output-dir") {
		t.Fatalf("expected actionable guidance, got %v", err)
	}
	if called {
		t.Fatal("expected runExtract to fail before building kubernetes client")
	}
}

func TestRunExtract_RejectsUnexpectedPositionalArgs(t *testing.T) {
	clientOrig := buildClientFunc
	defer func() {
		buildClientFunc = clientOrig
	}()

	called := false
	buildClientFunc = func(kubeContext string) (*apiextensionsclient.Clientset, error) {
		called = true
		return apiextensionsclient.NewForConfig(&rest.Config{Host: "https://example.invalid"})
	}

	err := runExtract([]string{"--output-dir", t.TempDir(), "upload"})
	if err == nil {
		t.Fatal("expected runExtract error")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("expected unexpected arguments error, got %v", err)
	}
	if called {
		t.Fatal("expected runExtract to fail before building kubernetes client")
	}
}

func TestRunConvert_WritesSchemas(t *testing.T) {
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}

	crdYAML := `apiVersion: apiextensions.k8s.io/v1
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
              properties:
                secretName:
                  type: string
`
	if err := os.WriteFile(filepath.Join(inputDir, "crd.yaml"), []byte(crdYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runConvert([]string{"--file", filepath.Join(inputDir, "crd.yaml"), "--output-dir", outputDir})
	if err != nil {
		t.Fatalf("runConvert error: %v", err)
	}

	schemaPath := filepath.Join(outputDir, "cert-manager.io", "certificate_v1.json")
	if _, err := os.Stat(schemaPath); err != nil {
		t.Fatalf("expected schema at %s: %v", schemaPath, err)
	}

	standalonePath := filepath.Join(outputDir, "master-standalone", "cert-manager.io-certificate-stable-v1.json")
	if _, err := os.Stat(standalonePath); err != nil {
		t.Fatalf("expected standalone schema at %s: %v", standalonePath, err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, ".generations")); !os.IsNotExist(err) {
		t.Fatal("convert should not create .generations directory")
	}
}

func TestRunConvert_WithDirFlag(t *testing.T) {
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}

	crdYAML := `apiVersion: apiextensions.k8s.io/v1
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
`
	if err := os.WriteFile(filepath.Join(inputDir, "crd.yaml"), []byte(crdYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runConvert([]string{"--dir", inputDir, "--output-dir", outputDir})
	if err != nil {
		t.Fatalf("runConvert error: %v", err)
	}

	schemaPath := filepath.Join(outputDir, "cert-manager.io", "certificate_v1.json")
	if _, err := os.Stat(schemaPath); err != nil {
		t.Fatalf("expected schema at %s: %v", schemaPath, err)
	}
}

func TestRunConvert_FiltersByKind(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")

	multiCRDYAML := `---
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
	inputFile := filepath.Join(dir, "crds.yaml")
	if err := os.WriteFile(inputFile, []byte(multiCRDYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir, "--kind", "certificate"})
	if err != nil {
		t.Fatalf("runConvert error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "certificate_v1.json")); err != nil {
		t.Fatal("expected Certificate schema to exist")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "issuer_v1.json")); !os.IsNotExist(err) {
		t.Fatal("expected Issuer schema to be filtered out")
	}
}

func TestRunConvert_CombinesFileAndDir(t *testing.T) {
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}

	certCRD := `apiVersion: apiextensions.k8s.io/v1
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
`
	issuerCRD := `apiVersion: apiextensions.k8s.io/v1
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
	singleFile := filepath.Join(dir, "cert.yaml")
	if err := os.WriteFile(singleFile, []byte(certCRD), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "issuer.yaml"), []byte(issuerCRD), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runConvert([]string{"--file", singleFile, "--dir", inputDir, "--output-dir", outputDir})
	if err != nil {
		t.Fatalf("runConvert error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "certificate_v1.json")); err != nil {
		t.Fatal("expected Certificate schema from --file")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "issuer_v1.json")); err != nil {
		t.Fatal("expected Issuer schema from --dir")
	}
}

func TestRunConvert_RequiresOutputDir(t *testing.T) {
	err := runConvert([]string{"--file", "crd.yaml"})
	if err == nil {
		t.Fatal("expected error when --output-dir is missing")
	}
	if !strings.Contains(err.Error(), "output-dir") {
		t.Errorf("expected error about output-dir, got: %v", err)
	}
}

func TestRunConvert_RequiresInput(t *testing.T) {
	err := runConvert([]string{"--output-dir", "/tmp/out"})
	if err == nil {
		t.Fatal("expected error when no input is specified")
	}
	if !strings.Contains(err.Error(), "file") || !strings.Contains(err.Error(), "dir") {
		t.Errorf("expected error mentioning --file or --dir, got: %v", err)
	}
}

func TestRunConvert_WithRender(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")

	crdYAML := `apiVersion: apiextensions.k8s.io/v1
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
	inputFile := filepath.Join(dir, "crd.yaml")
	if err := os.WriteFile(inputFile, []byte(crdYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir, "--render"})
	if err != nil {
		t.Fatalf("runConvert --render error: %v", err)
	}

	// JSON schema should exist
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "certificate_v1.json")); err != nil {
		t.Fatal("expected JSON schema file")
	}

	// HTML schema page should exist
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "certificate_v1.html")); err != nil {
		t.Fatal("expected rendered HTML schema page")
	}

	// Index page should exist
	if _, err := os.Stat(filepath.Join(outputDir, "index.html")); err != nil {
		t.Fatal("expected generated index.html")
	}
}

func TestRunConvert_CleansStaleSchemas(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")

	multiCRDYAML := `---
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
	inputFile := filepath.Join(dir, "crds.yaml")
	if err := os.WriteFile(inputFile, []byte(multiCRDYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// First run: both CRDs
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir}); err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "issuer_v1.json")); err != nil {
		t.Fatal("expected Issuer schema after first run")
	}

	// Second run: only Certificate — stale Issuer should be removed
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir, "--kind", "certificate"}); err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "certificate_v1.json")); err != nil {
		t.Fatal("expected Certificate schema after second run")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "issuer_v1.json")); !os.IsNotExist(err) {
		t.Fatal("expected Issuer schema to be cleaned up after filtered second run")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "master-standalone", "cert-manager.io-issuer-stable-v1.json")); !os.IsNotExist(err) {
		t.Fatal("expected standalone Issuer schema to be cleaned up")
	}
}

func TestRunConvert_CleansStaleOnEmptyFilter(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")

	crdYAML := `apiVersion: apiextensions.k8s.io/v1
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
`
	inputFile := filepath.Join(dir, "crd.yaml")
	if err := os.WriteFile(inputFile, []byte(crdYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// First run: generates Certificate schemas
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir}); err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "certificate_v1.json")); err != nil {
		t.Fatal("expected schema after first run")
	}

	// Second run: filter matches nothing — stale output should still be cleaned
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir, "--kind", "nonexistent"}); err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io")); !os.IsNotExist(err) {
		t.Fatal("expected stale group directory to be cleaned even when filter matches nothing")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "master-standalone")); !os.IsNotExist(err) {
		t.Fatal("expected stale master-standalone to be cleaned")
	}
}

func TestRunConvert_PreservesUserFiles(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create user files that should not be deleted — including a directory
	// with a dot in the name to verify we don't use dot-heuristics
	if err := os.WriteFile(filepath.Join(outputDir, "README.md"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outputDir, "my.scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "my.scripts", "validate.sh"), []byte("#!/bin/sh"), 0o644); err != nil {
		t.Fatal(err)
	}

	crdYAML := `apiVersion: apiextensions.k8s.io/v1
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
`
	inputFile := filepath.Join(dir, "crd.yaml")
	if err := os.WriteFile(inputFile, []byte(crdYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// First run
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir}); err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// Second run — user files must survive cleanup of the first run's manifest
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir}); err != nil {
		t.Fatalf("second run error: %v", err)
	}

	// Generated output should exist
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "certificate_v1.json")); err != nil {
		t.Fatal("expected generated schema")
	}

	// User files should be preserved across both runs
	data, err := os.ReadFile(filepath.Join(outputDir, "README.md"))
	if err != nil {
		t.Fatal("expected README.md to be preserved after two runs")
	}
	if string(data) != "keep me" {
		t.Fatalf("README.md content changed: %q", data)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "my.scripts", "validate.sh")); err != nil {
		t.Fatal("expected my.scripts/validate.sh to be preserved after two runs")
	}
}

func TestRunConvert_PreservesUserNamedFiles(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create user files with names that match render artifacts
	if err := os.WriteFile(filepath.Join(outputDir, "index.html"), []byte("my landing page"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "favicon.svg"), []byte("my icon"), 0o644); err != nil {
		t.Fatal(err)
	}

	crdYAML := `apiVersion: apiextensions.k8s.io/v1
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
`
	inputFile := filepath.Join(dir, "crd.yaml")
	if err := os.WriteFile(inputFile, []byte(crdYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// First run — no prior manifest, user files should survive
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir}); err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// Second run — manifest exists from first run, user files must still survive
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir}); err != nil {
		t.Fatalf("second run error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "index.html"))
	if err != nil {
		t.Fatal("expected user index.html to be preserved after two runs")
	}
	if string(data) != "my landing page" {
		t.Fatalf("index.html was overwritten: %q", data)
	}
	data, err = os.ReadFile(filepath.Join(outputDir, "favicon.svg"))
	if err != nil {
		t.Fatal("expected user favicon.svg to be preserved after two runs")
	}
	if string(data) != "my icon" {
		t.Fatalf("favicon.svg was overwritten: %q", data)
	}
}

func TestRunConvert_CorruptManifest_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")

	crdYAML := `apiVersion: apiextensions.k8s.io/v1
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
`
	inputFile := filepath.Join(dir, "crd.yaml")
	if err := os.WriteFile(inputFile, []byte(crdYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// First run produces output
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir}); err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// Corrupt the manifest
	if err := os.WriteFile(filepath.Join(outputDir, "_meta", "convert-manifest.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Second run should fail with a clear error about the corrupt manifest
	err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir})
	if err == nil {
		t.Fatal("expected error for corrupt manifest")
	}
	if !strings.Contains(err.Error(), "manifest") {
		t.Errorf("expected error mentioning manifest, got: %v", err)
	}
}

func TestRunConvert_PreservesUserFilesInsideGeneratedDirs(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")

	crdYAML := `apiVersion: apiextensions.k8s.io/v1
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
`
	inputFile := filepath.Join(dir, "crd.yaml")
	if err := os.WriteFile(inputFile, []byte(crdYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// First run creates cert-manager.io/
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir}); err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// User adds a file inside the generated directory
	if err := os.WriteFile(filepath.Join(outputDir, "cert-manager.io", "README.md"), []byte("user notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Second run should clean generated files but preserve the user's README.md
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir}); err != nil {
		t.Fatalf("second run error: %v", err)
	}

	// Generated schema should be freshly written
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "certificate_v1.json")); err != nil {
		t.Fatal("expected schema to exist after second run")
	}

	// User file inside generated directory should survive
	data, err := os.ReadFile(filepath.Join(outputDir, "cert-manager.io", "README.md"))
	if err != nil {
		t.Fatal("expected user README.md inside generated dir to be preserved")
	}
	if string(data) != "user notes" {
		t.Fatalf("README.md content changed: %q", data)
	}
}

func TestRunConvert_CleansRenderArtifacts(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")

	crdYAML := `apiVersion: apiextensions.k8s.io/v1
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
`
	inputFile := filepath.Join(dir, "crd.yaml")
	if err := os.WriteFile(inputFile, []byte(crdYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// First run with --render
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir, "--render"}); err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "index.html")); err != nil {
		t.Fatal("expected index.html after render run")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "favicon.svg")); err != nil {
		t.Fatal("expected favicon.svg after render run")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "schema-search.js")); err != nil {
		t.Fatal("expected schema-search.js after render run")
	}

	// Second run without --render — all render artifacts should be cleaned
	if err := runConvert([]string{"--file", inputFile, "--output-dir", outputDir}); err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "index.html")); !os.IsNotExist(err) {
		t.Fatal("expected index.html to be cleaned after non-render run")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "favicon.svg")); !os.IsNotExist(err) {
		t.Fatal("expected favicon.svg to be cleaned after non-render run")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "schema-search.js")); !os.IsNotExist(err) {
		t.Fatal("expected schema-search.js to be cleaned after non-render run")
	}
	// JSON schema should still be present
	if _, err := os.Stat(filepath.Join(outputDir, "cert-manager.io", "certificate_v1.json")); err != nil {
		t.Fatal("expected schema to exist after non-render run")
	}
}

func TestRunConvert_ValidatesOutputDir(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "crd.yaml")
	if err := os.WriteFile(inputFile, []byte("apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Current working directory should be rejected
	err := runConvert([]string{"--file", inputFile, "--output-dir", "."})
	if err == nil {
		t.Fatal("expected error for unsafe output-dir '.'")
	}
}
