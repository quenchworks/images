package common

// path walks decoded JSON: strings index objects, ints index arrays.
func Path(d any, keys ...any) any {
	for _, k := range keys {
		switch k := k.(type) {
		case string:
			m, _ := d.(map[string]any)
			d = m[k]
		case int:
			a, _ := d.([]any)
			if k >= len(a) {
				return nil
			}
			d = a[k]
		}
	}
	return d
}

func Str(d any) string { s, _ := d.(string); return s }

// each maps every element of a JSON array through f, dropping empty results.
func Each(d any, f func(any) string) []string {
	a, _ := d.([]any)
	var out []string
	for _, e := range a {
		if s := f(e); s != "" {
			out = append(out, s)
		}
	}
	return out
}
