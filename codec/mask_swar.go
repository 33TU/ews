//go:build (!goexperiment.simd && (amd64 || arm64 || wasm)) || 386

package codec

func mask(dst, src []byte, key [4]byte, offset uint8) { maskSWAR(dst, src, key, offset) }
