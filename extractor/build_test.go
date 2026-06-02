package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sholdee/crd-schema-publisher/diagnostics"
	"github.com/sholdee/crd-schema-publisher/schemametadata"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

const buildSiteOpenAPI = `{
  "definitions": {
    "io.k8s.api.core.v1.Pod": {
      "type": "object",
      "x-kubernetes-group-version-kind": [{"group": "", "version": "v1", "kind": "Pod"}],
      "properties": {
        "apiVersion": {"type": "string"},
        "kind": {"type": "string"}
      }
    },
    "io.k8s.api.apps.v1.Deployment": {
      "type": "object",
      "x-kubernetes-group-version-kind": [{"group": "apps", "version": "v1", "kind": "Deployment"}],
      "properties": {
        "apiVersion": {"type": "string"},
        "kind": {"type": "string"}
      }
    }
  }
}`

const buildSiteOpenAPIWithCRDs = `{
  "definitions": {
    "io.k8s.api.core.v1.Pod": {
      "type": "object",
      "x-kubernetes-group-version-kind": [{"group": "", "version": "v1", "kind": "Pod"}],
      "properties": {
        "apiVersion": {"type": "string"},
        "kind": {"type": "string"}
      }
    },
    "io.example.v1.Test": {
      "type": "object",
      "x-kubernetes-group-version-kind": [{"group": "example.io", "version": "v1", "kind": "Test"}],
      "properties": {
        "apiVersion": {"type": "string"},
        "kind": {"type": "string"},
        "openapiOnly": {"type": "string"}
      }
    },
    "io.example.v1.TestList": {
      "type": "object",
      "x-kubernetes-group-version-kind": [{"group": "example.io", "version": "v1", "kind": "TestList"}],
      "properties": {
        "apiVersion": {"type": "string"},
        "kind": {"type": "string"},
        "items": {"type": "array"}
      }
    }
  }
}`

type fakeOpenAPISource struct {
	raw []byte
	err error
}

func (f fakeOpenAPISource) FetchOpenAPIV2(context.Context) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.raw, nil
}

type recordingSnapshotter struct {
	phases []string
}

func (r *recordingSnapshotter) Snapshot(phase string, attrs ...any) {
	r.phases = append(r.phases, phase)
}

func seedActiveGeneration(t *testing.T, outputDir string, files map[string]string) string {
	t.Helper()

	generationsDir := filepath.Join(outputDir, ".generations")
	if err := os.MkdirAll(generationsDir, 0o755); err != nil {
		t.Fatalf("creating generations dir: %v", err)
	}

	generationDir := filepath.Join(generationsDir, "seed")
	if err := os.MkdirAll(generationDir, 0o755); err != nil {
		t.Fatalf("creating generation dir: %v", err)
	}
	for rel, content := range files {
		path := filepath.Join(generationDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating parent dir for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("writing %s: %v", rel, err)
		}
	}

	currentPath := filepath.Join(outputDir, "current")
	if err := os.Symlink(filepath.Join(".generations", "seed"), currentPath); err != nil {
		t.Fatalf("creating current symlink: %v", err)
	}

	return generationDir
}

func TestBuildSite_ProfilesBuildPhases(t *testing.T) {
	outputDir := t.TempDir()
	profiler := &recordingSnapshotter{}

	result, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
		Render:    true,
		Profiler:  profiler,
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.Status != BuildResultBuilt {
		t.Fatalf("expected BuildResultBuilt, got %q", result.Status)
	}

	for _, phase := range []string{
		"build.start",
		"build.after-list-crds",
		"build.after-filter-crds",
		"build.after-write-schemas",
		"build.after-render",
		"build.after-index",
		"build.after-activate",
		"build.after-clean-legacy-root",
		"build.after-prune-generations",
	} {
		if !slices.Contains(profiler.phases, phase) {
			t.Fatalf("expected profile phase %q in %v", phase, profiler.phases)
		}
	}
}

func TestBuildSite_PreservesProfilesUnderOutputDir(t *testing.T) {
	outputDir := t.TempDir()
	profileDir := filepath.Join(outputDir, "profile")

	_, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
		Render:    true,
		Profiler:  diagnostics.NewProfiler(profileDir),
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(profileDir, "*-heap-build-after-render.pprof"))
	if err != nil {
		t.Fatalf("glob profile dir: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected build.after-render profile to survive cleanup under %s", profileDir)
	}
}

func currentTarget(t *testing.T, outputDir string) string {
	t.Helper()

	target, err := os.Readlink(filepath.Join(outputDir, "current"))
	if err != nil {
		t.Fatalf("reading current symlink: %v", err)
	}
	return target
}

func TestValidateOutputDir(t *testing.T) {
	t.Run("rejects catastrophic targets", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}

		rootLink := filepath.Join(t.TempDir(), "root-link")
		if err := os.Symlink(string(filepath.Separator), rootLink); err != nil {
			t.Fatalf("symlink root: %v", err)
		}

		tests := []struct {
			name  string
			input string
		}{
			{name: "empty", input: ""},
			{name: "dot", input: "."},
			{name: "dotdot", input: ".."},
			{name: "root", input: string(filepath.Separator)},
			{name: "cwd", input: cwd},
			{name: "symlinked root", input: rootLink},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := ValidateOutputDir(tt.input); err == nil {
					t.Fatalf("ValidateOutputDir(%q) expected error", tt.input)
				}
			})
		}
	})

	t.Run("allows dedicated directories", func(t *testing.T) {
		for _, dir := range []string{
			filepath.Join(t.TempDir(), "output"),
			filepath.Join("testdata", "output"),
		} {
			if err := ValidateOutputDir(dir); err != nil {
				t.Fatalf("ValidateOutputDir(%q) unexpected error: %v", dir, err)
			}
		}
	})
}

func TestBuildSite_ZeroCRDsIsNoopAndPreservesOutput(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "keep",
	})
	before := currentTarget(t, outputDir)

	result, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: nil},
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.Status != BuildResultNoop {
		t.Fatalf("expected BuildResultNoop, got %q", result.Status)
	}
	if got := currentTarget(t, outputDir); got != before {
		t.Fatalf("expected current to remain %q, got %q", before, got)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected active generation to remain readable: %v", err)
	}
}

func TestBuildSite_UsesContextForCRDList(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := BuildSite(SiteBuildOptions{
		Context:   ctx,
		Lister:    &fakeLister{},
		OutputDir: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("expected canceled context error, got %v", err)
	}
}

func TestBuildSite_IncludeBuiltinsWithZeroCRDsBuildsGeneration(t *testing.T) {
	outputDir := t.TempDir()

	result, err := BuildSite(SiteBuildOptions{
		Lister:          &fakeLister{},
		OutputDir:       outputDir,
		IncludeBuiltins: true,
		OpenAPISource:   fakeOpenAPISource{raw: []byte(buildSiteOpenAPI)},
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.Status != BuildResultBuilt {
		t.Fatalf("expected BuildResultBuilt, got %q", result.Status)
	}
	if result.CRDCount != 0 || result.SchemaCount != 2 {
		t.Fatalf("expected CRDCount=0 SchemaCount=2, got CRDCount=%d SchemaCount=%d", result.CRDCount, result.SchemaCount)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "core", "pod_v1.json")); err != nil {
		t.Fatalf("expected Pod schema: %v", err)
	}
}

func TestBuildSite_IncludeBuiltinsHonorsFilter(t *testing.T) {
	outputDir := t.TempDir()

	result, err := BuildSite(SiteBuildOptions{
		Lister:          &fakeLister{},
		OutputDir:       outputDir,
		Filter:          ParseFilter("pod", "", ""),
		IncludeBuiltins: true,
		OpenAPISource:   fakeOpenAPISource{raw: []byte(buildSiteOpenAPI)},
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.SchemaCount != 1 {
		t.Fatalf("expected one filtered schema, got %d", result.SchemaCount)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "core", "pod_v1.json")); err != nil {
		t.Fatalf("expected Pod schema: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "apps", "deployment_v1.json")); !os.IsNotExist(err) {
		t.Fatalf("expected Deployment schema to be filtered out, got err=%v", err)
	}
}

func TestBuildSite_IncludeKustomizeWithZeroCRDsBuildsGeneration(t *testing.T) {
	outputDir := t.TempDir()

	result, err := BuildSite(SiteBuildOptions{
		Lister:           &fakeLister{},
		OutputDir:        outputDir,
		IncludeKustomize: true,
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.Status != BuildResultBuilt {
		t.Fatalf("expected BuildResultBuilt, got %q", result.Status)
	}
	if result.CRDCount != 0 || result.SchemaCount != 1 {
		t.Fatalf("expected CRDCount=0 SchemaCount=1, got CRDCount=%d SchemaCount=%d", result.CRDCount, result.SchemaCount)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "kustomize.config.k8s.io", "kustomization_v1beta1.json")); err != nil {
		t.Fatalf("expected Kustomization schema: %v", err)
	}
}

func TestBuildSite_MergesCRDsBuiltinsAndKustomizeKindsManifest(t *testing.T) {
	outputDir := t.TempDir()

	result, err := BuildSite(SiteBuildOptions{
		Lister:           &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir:        outputDir,
		IncludeBuiltins:  true,
		IncludeKustomize: true,
		OpenAPISource:    fakeOpenAPISource{raw: []byte(buildSiteOpenAPI)},
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.CRDCount != 1 || result.SchemaCount != 4 {
		t.Fatalf("expected CRDCount=1 SchemaCount=4, got CRDCount=%d SchemaCount=%d", result.CRDCount, result.SchemaCount)
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "current", "_meta", "kinds.json"))
	if err != nil {
		t.Fatalf("reading kinds manifest: %v", err)
	}
	var kinds map[string]string
	if err := json.Unmarshal(data, &kinds); err != nil {
		t.Fatal(err)
	}
	for path, kind := range map[string]string{
		"example.io/test_v1.json":                            "Test",
		"core/pod_v1.json":                                   "Pod",
		"kustomize.config.k8s.io/kustomization_v1beta1.json": "Kustomization",
		"apps/deployment_v1.json":                            "Deployment",
	} {
		if kinds[path] != kind {
			t.Fatalf("expected kinds[%q]=%q, got %q in %v", path, kind, kinds[path], kinds)
		}
	}
}

func TestBuildSite_IncludeBuiltinsSkipsOpenAPICRDDefinitions(t *testing.T) {
	outputDir := t.TempDir()

	result, err := BuildSite(SiteBuildOptions{
		Lister:          &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir:       outputDir,
		IncludeBuiltins: true,
		OpenAPISource:   fakeOpenAPISource{raw: []byte(buildSiteOpenAPIWithCRDs)},
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.CRDCount != 1 || result.SchemaCount != 2 {
		t.Fatalf("expected CRDCount=1 SchemaCount=2, got CRDCount=%d SchemaCount=%d", result.CRDCount, result.SchemaCount)
	}
	assertCRDPreferredOverOpenAPI(t, filepath.Join(outputDir, "current"))
}

func assertCRDPreferredOverOpenAPI(t *testing.T, dir string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "example.io", "test_v1.json"))
	if err != nil {
		t.Fatalf("reading CRD schema: %v", err)
	}
	if !strings.Contains(string(data), `"spec"`) {
		t.Fatalf("expected CRD schema to remain in primary path, got:\n%s", data)
	}
	if strings.Contains(string(data), "openapiOnly") {
		t.Fatalf("expected OpenAPI CRD definition not to overwrite primary CRD schema:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "example.io", "testlist_v1.json")); !os.IsNotExist(err) {
		t.Fatalf("expected CRD List OpenAPI schema to be skipped, got err=%v", err)
	}
	metadata := readSchemaMetadata(t, dir)
	if got := metadata["example.io/test_v1.json"].Source; got != schemametadata.SchemaSourceCRD {
		t.Fatalf("expected CRD metadata source, got %q in %v", got, metadata)
	}
	if got := metadata["core/pod_v1.json"].Source; got != schemametadata.SchemaSourceBuiltin {
		t.Fatalf("expected Pod metadata source builtin, got %q in %v", got, metadata)
	}
	if _, ok := metadata["example.io/testlist_v1.json"]; ok {
		t.Fatalf("expected CRD List metadata to be skipped, got %v", metadata)
	}
}

func TestBuildSite_BuiltinFetchFailurePreservesPreviousOutput(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "keep",
	})
	before := currentTarget(t, outputDir)

	_, err := BuildSite(SiteBuildOptions{
		Lister:          &fakeLister{},
		OutputDir:       outputDir,
		IncludeBuiltins: true,
		OpenAPISource:   fakeOpenAPISource{err: fmt.Errorf("openapi unavailable")},
	})
	if err == nil {
		t.Fatal("expected BuildSite error")
	}
	if !strings.Contains(err.Error(), "fetching built-in OpenAPI") {
		t.Fatalf("expected built-in fetch error, got %v", err)
	}
	if got := currentTarget(t, outputDir); got != before {
		t.Fatalf("expected current to remain %q, got %q", before, got)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected prior active generation to remain readable: %v", err)
	}
}

func TestBuildSite_FilterNoMatchesBuildsEmptyGenerationAndSwitchesCurrent(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html":               "old index",
		"example.io/stale_v1.json": "{}",
	})
	before := currentTarget(t, outputDir)

	result, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
		Filter:    ParseFilter("missing", "", ""),
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.Status != BuildResultBuilt {
		t.Fatalf("expected BuildResultBuilt, got %q", result.Status)
	}
	if result.CRDCount != 0 || result.SchemaCount != 0 {
		t.Fatalf("expected empty filtered result, got CRDCount=%d SchemaCount=%d", result.CRDCount, result.SchemaCount)
	}
	after := currentTarget(t, outputDir)
	if after == before {
		t.Fatalf("expected current to switch from stale generation %q", before)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected empty generation index: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "example.io", "stale_v1.json")); !os.IsNotExist(err) {
		t.Fatalf("expected stale schema to be absent from empty filtered generation, got err=%v", err)
	}
}

func TestBuildSite_SuccessCreatesGenerationAndSwitchesCurrent(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "old",
	})
	before := currentTarget(t, outputDir)

	result, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.Status != BuildResultBuilt {
		t.Fatalf("expected BuildResultBuilt, got %q", result.Status)
	}
	after := currentTarget(t, outputDir)
	if after == before {
		t.Fatalf("expected current to switch generations, stayed at %q", before)
	}
	if !strings.HasPrefix(after, ".generations") {
		t.Fatalf("expected current target under .generations, got %q", after)
	}
	targetInfo, err := os.Stat(filepath.Join(outputDir, after))
	if err != nil {
		t.Fatalf("stat current target: %v", err)
	}
	if got := targetInfo.Mode().Perm(); got != 0o755 {
		t.Fatalf("expected active generation dir perms 0755, got %#o", got)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "example.io", "test_v1.json")); err != nil {
		t.Fatalf("expected schema output under current: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected index output under current: %v", err)
	}
}

func TestBuildSite_SuccessPrunesSupersededGenerations(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "previous",
	})

	staleDir := filepath.Join(outputDir, ".generations", "stale")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("mkdir stale generation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "index.html"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale index: %v", err)
	}

	result, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("BuildSite error: %v", err)
	}
	if result.Status != BuildResultBuilt {
		t.Fatalf("expected BuildResultBuilt, got %q", result.Status)
	}

	generations, err := os.ReadDir(filepath.Join(outputDir, ".generations"))
	if err != nil {
		t.Fatalf("read generations dir: %v", err)
	}
	if len(generations) != 2 {
		names := make([]string, 0, len(generations))
		for _, generation := range generations {
			names = append(names, generation.Name())
		}
		t.Fatalf("expected current and previous generations only, got %v", names)
	}
	for _, generation := range generations {
		if generation.Name() == "stale" {
			t.Fatal("expected stale superseded generation to be pruned")
		}
	}
}

func TestBuildSite_FailurePreservesPreviousOutput(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "keep",
	})
	before := currentTarget(t, outputDir)

	orig := generateIndexFunc
	generateIndexFunc = func(string, string) error {
		return fmt.Errorf("boom")
	}
	defer func() {
		generateIndexFunc = orig
	}()

	_, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
	})
	if err == nil {
		t.Fatal("expected BuildSite error")
	}
	if !strings.Contains(err.Error(), "generating index") {
		t.Fatalf("expected index error, got %v", err)
	}
	if got := currentTarget(t, outputDir); got != before {
		t.Fatalf("expected current to remain %q, got %q", before, got)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected prior active generation to remain readable: %v", err)
	}
}

func TestBuildSite_PostActivationFailureKeepsActiveGenerationReadable(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "previous",
	})
	before := currentTarget(t, outputDir)

	orig := pruneGenerationsFunc
	pruneGenerationsFunc = func(string, ...string) error {
		return fmt.Errorf("prune boom")
	}
	defer func() {
		pruneGenerationsFunc = orig
	}()

	_, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
	})
	if err == nil {
		t.Fatal("expected BuildSite error")
	}
	if !strings.Contains(err.Error(), "pruning generations") {
		t.Fatalf("expected pruning error, got %v", err)
	}

	after := currentTarget(t, outputDir)
	if after == before {
		t.Fatalf("expected current to switch generations, stayed at %q", before)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected activated generation to remain readable: %v", err)
	}
}

func TestBuildSite_RenderFailurePreservesPreviousOutput(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "keep",
	})
	before := currentTarget(t, outputDir)

	orig := renderAllFunc
	renderAllFunc = func(string, string) error {
		return fmt.Errorf("render boom")
	}
	defer func() {
		renderAllFunc = orig
	}()

	_, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
		Render:    true,
	})
	if err == nil {
		t.Fatal("expected BuildSite error")
	}
	if !strings.Contains(err.Error(), "rendering schemas") {
		t.Fatalf("expected render error, got %v", err)
	}
	if got := currentTarget(t, outputDir); got != before {
		t.Fatalf("expected current to remain %q, got %q", before, got)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected prior active generation to remain readable: %v", err)
	}
}

func TestBuildSite_WriteFailurePreservesPreviousOutput(t *testing.T) {
	outputDir := t.TempDir()
	seedActiveGeneration(t, outputDir, map[string]string{
		"index.html": "keep",
	})
	before := currentTarget(t, outputDir)

	orig := writeSchemasFunc
	writeSchemasFunc = func([]apiextensionsv1.CustomResourceDefinition, string) (int, error) {
		return 0, fmt.Errorf("write boom")
	}
	defer func() {
		writeSchemasFunc = orig
	}()

	_, err := BuildSite(SiteBuildOptions{
		Lister:    &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}},
		OutputDir: outputDir,
	})
	if err == nil {
		t.Fatal("expected BuildSite error")
	}
	if !strings.Contains(err.Error(), "writing schemas") {
		t.Fatalf("expected write error, got %v", err)
	}
	if got := currentTarget(t, outputDir); got != before {
		t.Fatalf("expected current to remain %q, got %q", before, got)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "current", "index.html")); err != nil {
		t.Fatalf("expected prior active generation to remain readable: %v", err)
	}
}
