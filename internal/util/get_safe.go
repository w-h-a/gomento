package util

func GetSafeInt(info map[string]any, key string) int {
	if info == nil {
		return 0
	}

	val, ok := info[key]
	if !ok {
		return 0
	}

	switch v := val.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	default:
		return 0
	}
}
