// Package utf8 validates UTF-8 faster than the standard library. The ASCII
// prefix is skipped with word loads. The rest uses a shift-based DFA, or
// SIMD lookups under GOEXPERIMENT=simd on amd64.
package utf8
