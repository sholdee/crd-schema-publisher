package extractor

import (
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

type SchemaFilter struct {
	Kinds    []string
	Groups   []string
	Versions []string
}

func ParseFilter(kinds, groups, versions string) SchemaFilter {
	return SchemaFilter{
		Kinds:    parseCSV(kinds),
		Groups:   parseCSV(groups),
		Versions: parseCSV(versions),
	}
}

func parseCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (f SchemaFilter) Matches(kind, group, version string) bool {
	return matchesAny(f.Kinds, kind) && matchesAny(f.Groups, group) && matchesAny(f.Versions, version)
}

func matchesAny(filter []string, value string) bool {
	if len(filter) == 0 {
		return true
	}
	lower := strings.ToLower(value)
	for _, f := range filter {
		if f == lower {
			return true
		}
	}
	return false
}

func FilterCRDs(crds []apiextensionsv1.CustomResourceDefinition, f SchemaFilter) []apiextensionsv1.CustomResourceDefinition {
	if f.Kinds == nil && f.Groups == nil && f.Versions == nil {
		return crds
	}

	var result []apiextensionsv1.CustomResourceDefinition
	for _, crd := range crds {
		if !matchesAny(f.Kinds, crd.Spec.Names.Kind) {
			continue
		}
		if !matchesAny(f.Groups, crd.Spec.Group) {
			continue
		}

		if f.Versions == nil {
			result = append(result, crd)
			continue
		}

		var matchedVersions []apiextensionsv1.CustomResourceDefinitionVersion
		for _, v := range crd.Spec.Versions {
			if matchesAny(f.Versions, v.Name) {
				matchedVersions = append(matchedVersions, v)
			}
		}
		if len(matchedVersions) > 0 {
			filtered := crd
			filtered.Spec.Versions = matchedVersions
			result = append(result, filtered)
		}
	}
	return result
}
