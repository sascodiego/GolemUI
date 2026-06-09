package dataaccess

// ExtractOrderedArgs maps snapshot keys to positional args in filterKeys order.
// Missing keys default to empty string (so LIKE ” matches everything instead of NULL = false).
func ExtractOrderedArgs(snap map[string]any, filterKeys []string) []any {
	if len(filterKeys) == 0 {
		return []any{}
	}
	args := make([]any, 0, len(filterKeys))
	for _, key := range filterKeys {
		if snap == nil {
			args = append(args, "")
			continue
		}
		val, exists := snap[key]
		if !exists {
			args = append(args, "")
		} else {
			args = append(args, val)
		}
	}
	return args
}
