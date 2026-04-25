package extractor

import (
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseFilter_LowercasesValues(t *testing.T) {
	f := ParseFilter("Certificate,Issuer", "Cert-Manager.IO", "V1")
	if f.Kinds[0] != "certificate" || f.Kinds[1] != "issuer" {
		t.Errorf("expected lowercased kinds, got %v", f.Kinds)
	}
	if f.Groups[0] != "cert-manager.io" {
		t.Errorf("expected lowercased group, got %v", f.Groups)
	}
	if f.Versions[0] != "v1" {
		t.Errorf("expected lowercased version, got %v", f.Versions)
	}
}

func TestParseFilter_EmptyStringsProduceNilSlices(t *testing.T) {
	f := ParseFilter("", "", "")
	if f.Kinds != nil || f.Groups != nil || f.Versions != nil {
		t.Errorf("expected nil slices for empty input, got kinds=%v groups=%v versions=%v", f.Kinds, f.Groups, f.Versions)
	}
}

func TestSchemaFilter_MatchesAll_WhenEmpty(t *testing.T) {
	f := ParseFilter("", "", "")
	if !f.Matches("Certificate", "cert-manager.io", "v1") {
		t.Error("empty filter should match everything")
	}
}

func TestSchemaFilter_MatchesKind_CaseInsensitive(t *testing.T) {
	f := ParseFilter("certificate", "", "")
	if !f.Matches("Certificate", "cert-manager.io", "v1") {
		t.Error("should match Certificate with lowercase filter")
	}
	if f.Matches("Issuer", "cert-manager.io", "v1") {
		t.Error("should not match Issuer")
	}
}

func TestSchemaFilter_ORWithinType_ANDAcrossTypes(t *testing.T) {
	f := ParseFilter("certificate,issuer", "cert-manager.io", "")
	if !f.Matches("Certificate", "cert-manager.io", "v1") {
		t.Error("should match Certificate in cert-manager.io")
	}
	if !f.Matches("Issuer", "cert-manager.io", "v1") {
		t.Error("should match Issuer in cert-manager.io")
	}
	if f.Matches("Certificate", "monitoring.coreos.com", "v1") {
		t.Error("should not match Certificate in wrong group")
	}
}

func TestSchemaFilter_VersionFilter(t *testing.T) {
	f := ParseFilter("", "", "v1,v2")
	if !f.Matches("Certificate", "cert-manager.io", "v1") {
		t.Error("should match v1")
	}
	if !f.Matches("Certificate", "cert-manager.io", "v2") {
		t.Error("should match v2")
	}
	if f.Matches("Certificate", "cert-manager.io", "v1beta1") {
		t.Error("should not match v1beta1")
	}
}

func testCRD(kind, group string, versions ...string) apiextensionsv1.CustomResourceDefinition {
	crd := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: kind + "." + group},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: kind},
		},
	}
	for _, v := range versions {
		crd.Spec.Versions = append(crd.Spec.Versions, apiextensionsv1.CustomResourceDefinitionVersion{
			Name: v,
			Schema: &apiextensionsv1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object"},
			},
		})
	}
	return crd
}

func TestFilterCRDs_FiltersVersions(t *testing.T) {
	crds := []apiextensionsv1.CustomResourceDefinition{
		testCRD("Certificate", "cert-manager.io", "v1", "v1alpha2"),
		testCRD("Issuer", "cert-manager.io", "v1"),
		testCRD("Prometheus", "monitoring.coreos.com", "v1"),
	}
	f := ParseFilter("certificate", "", "v1")
	result := FilterCRDs(crds, f)

	if len(result) != 1 {
		t.Fatalf("expected 1 CRD, got %d", len(result))
	}
	if result[0].Spec.Names.Kind != "Certificate" {
		t.Errorf("expected Certificate, got %s", result[0].Spec.Names.Kind)
	}
	if len(result[0].Spec.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(result[0].Spec.Versions))
	}
	if result[0].Spec.Versions[0].Name != "v1" {
		t.Errorf("expected v1, got %s", result[0].Spec.Versions[0].Name)
	}
}

func TestFilterCRDs_EmptyFilter_ReturnsAll(t *testing.T) {
	crds := []apiextensionsv1.CustomResourceDefinition{
		testCRD("Certificate", "cert-manager.io", "v1"),
		testCRD("Prometheus", "monitoring.coreos.com", "v1"),
	}
	f := ParseFilter("", "", "")
	result := FilterCRDs(crds, f)
	if len(result) != 2 {
		t.Fatalf("expected 2 CRDs, got %d", len(result))
	}
}

func TestFilterCRDs_NoMatch_ReturnsEmpty(t *testing.T) {
	crds := []apiextensionsv1.CustomResourceDefinition{
		testCRD("Certificate", "cert-manager.io", "v1"),
	}
	f := ParseFilter("nonexistent", "", "")
	result := FilterCRDs(crds, f)
	if len(result) != 0 {
		t.Fatalf("expected 0 CRDs, got %d", len(result))
	}
}
