package common

import (
	"fmt"
	"strings"
)

// pyList prints like Python's repr of a list of strings, so the output matches the old checkers.
func PyList(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = "'" + v + "'"
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func PyRepr(v any) string {
	if v == nil {
		return "None"
	}
	if s, ok := v.(string); ok {
		return "'" + s + "'"
	}
	return fmt.Sprint(v)
}

func ContainsAny(s string, subs []string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}
