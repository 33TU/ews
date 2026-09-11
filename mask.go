package ews

func mask(dst, src []byte, key [4]byte) {
	for i, v := range src {
		dst[i] = v ^ key[i&3]
	}
}
