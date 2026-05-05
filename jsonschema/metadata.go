package jsonschema

func RequiredSet(required []string) map[string]struct{} {
	set := make(map[string]struct{}, len(required))
	for _, name := range required {
		set[name] = struct{}{}
	}
	return set
}

func IsRequired(required map[string]struct{}, name string) bool {
	_, ok := required[name]
	return ok
}

func UniqueNonNullBranchTypes[T any](branches []T, display func(T) string) []string {
	types := make([]string, 0, len(branches))
	seen := make(map[string]bool, len(branches))
	for _, branch := range branches {
		typ := display(branch)
		if typ == "null" || seen[typ] {
			continue
		}
		seen[typ] = true
		types = append(types, typ)
	}
	return types
}
