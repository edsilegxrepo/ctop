// Package jsonfmt detects and pretty-prints structured JSON log entries into readable dot-notation key-value pairs.
//
// Objective:
//
//	Transform minified or nested JSON log payloads into aligned, human-readable terminal/log strings while passing raw text through unmodified.
//
// Functionality:
//   - IsJSON: High-speed heuristic check for JSON structure.
//   - FormatLogMessage: Unmarshals and flattens nested JSON hierarchies into dot notation (e.g. `http.status=200`).
package jsonfmt

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// IsJSON checks if a trimmed string starts and ends like a JSON object.
func IsJSON(s string) bool {
	trimmed := strings.TrimSpace(s)
	return strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")
}

// FormatLogMessage checks if the message is JSON; if so, flattens and formats it into colorized/aligned key-value pairs.
// If it is not JSON, the original message is returned unmodified.
func FormatLogMessage(msg string) string {
	trimmed := strings.TrimSpace(msg)
	if !IsJSON(trimmed) {
		return msg
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return msg
	}

	kvs := flattenJSON("", raw)
	if len(kvs) == 0 {
		return msg
	}

	// Sort keys alphabetically for predictable log viewing
	keys := make([]string, 0, len(kvs))
	for k := range kvs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("  ")
		}
		val := kvs[k]
		fmt.Fprintf(&sb, "%s=%s", k, formatValue(val))
	}

	return sb.String()
}

func flattenJSON(prefix string, obj map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range obj {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}

		switch child := v.(type) {
		case map[string]interface{}:
			nested := flattenJSON(fullKey, child)
			for nk, nv := range nested {
				result[nk] = nv
			}
		case []interface{}:
			for idx, item := range child {
				itemKey := fmt.Sprintf("%s[%d]", fullKey, idx)
				if itemMap, ok := item.(map[string]interface{}); ok {
					nested := flattenJSON(itemKey, itemMap)
					for nk, nv := range nested {
						result[nk] = nv
					}
				} else {
					result[itemKey] = item
				}
			}
		default:
			result[fullKey] = v
		}
	}
	return result
}

func formatValue(val interface{}) string {
	if val == nil {
		return "null"
	}
	switch v := val.(type) {
	case string:
		if strings.ContainsAny(v, " \t\n\"") {
			return fmt.Sprintf("\"%s\"", v)
		}
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}
