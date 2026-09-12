//go:build !(goexperiment.simd && amd64)

package utf8

import "unicode/utf8"

// Valid reports whether b is entirely valid UTF-8.
func Valid(b []byte) bool {
	return utf8.Valid(b)
}
