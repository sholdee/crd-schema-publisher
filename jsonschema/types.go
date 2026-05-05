package jsonschema

func TypeIncludes(t any, target string) bool {
	if t == "null" {
		return target == "null"
	}
	if t == target {
		return true
	}
	types, ok := t.([]any)
	if !ok {
		return false
	}
	for _, typ := range types {
		if typ == target {
			return true
		}
	}
	return false
}

func TypeIncludesNull(t any) bool {
	return TypeIncludes(t, "null")
}

func NonNullType(t any) string {
	switch typ := t.(type) {
	case string:
		return typ
	case []any:
		for _, v := range typ {
			if s, ok := v.(string); ok && s != "null" {
				return s
			}
		}
	}
	return ""
}
