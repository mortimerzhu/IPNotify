package notifier

import "fmt"

// String extracts a string value from a config map. If required and missing or
// empty, it returns an error.
func String(cfg map[string]any, key string, required bool) (string, error) {
	v, ok := cfg[key]
	if !ok || v == nil {
		if required {
			return "", fmt.Errorf("missing required field %q", key)
		}
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("field %q must be a string", key)
	}
	if required && s == "" {
		return "", fmt.Errorf("field %q must not be empty", key)
	}
	return s, nil
}

// StringMap extracts a map[string]string from a config map. Missing keys yield
// an empty map. Values are stringified.
func StringMap(cfg map[string]any, key string) (map[string]string, error) {
	v, ok := cfg[key]
	if !ok || v == nil {
		return map[string]string{}, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("field %q must be a map", key)
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		out[k] = fmt.Sprintf("%v", val)
	}
	return out, nil
}
