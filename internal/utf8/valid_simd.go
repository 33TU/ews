//go:build goexperiment.simd && amd64

package utf8

import (
	"simd/archsimd"
	"unicode/utf8"
	"unsafe"
)

// wideThreshold is the input size from which the 256-bit kernel pays off.
const wideThreshold = 384

var (
	utf8FirstHigh = [16]byte{
		0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
		0x80, 0x80, 0x80, 0x80, 0x21, 0x01, 0x15, 0x49,
	}
	utf8FirstLow = [16]byte{
		0xe7, 0xa3, 0x83, 0x83, 0x8b, 0xcb, 0xcb, 0xcb,
		0xcb, 0xcb, 0xcb, 0xcb, 0xcb, 0xdb, 0xcb, 0xcb,
	}
	utf8SecondHigh = [16]byte{
		0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
		0xe6, 0xae, 0xba, 0xba, 0x01, 0x01, 0x01, 0x01,
	}
)

// Valid reports whether src is entirely valid UTF-8. The ASCII prefix is
// skipped with word loads; the rest uses 128-bit lookups, or 256-bit ones
// with AVX2 on inputs of at least wideThreshold bytes. The lookup kernel
// needs byte permutes that archsimd offers only on amd64. Ported from
// github.com/33TU/json-experiment.
func Valid(src []byte) bool {
	// ASCII is the common case and the lookup kernel is slower on it than
	// plain word loads, so skip the ASCII prefix first, 32, 16, then 8 bytes
	// at a time. The kernel treats a skipped prefix as preceding ASCII, which
	// is exactly what it is.
	const high = 0x8080808080808080
	data := unsafe.Pointer(unsafe.SliceData(src))
	i := 0
	for i+32 <= len(src) {
		words := *(*uint64)(unsafe.Add(data, i)) | *(*uint64)(unsafe.Add(data, i+8)) |
			*(*uint64)(unsafe.Add(data, i+16)) | *(*uint64)(unsafe.Add(data, i+24))
		if words&high != 0 {
			break
		}
		i += 32
	}
	if i+16 <= len(src) && (*(*uint64)(unsafe.Add(data, i))|*(*uint64)(unsafe.Add(data, i+8)))&high == 0 {
		i += 16
	}
	if i+8 <= len(src) && *(*uint64)(unsafe.Add(data, i))&high == 0 {
		i += 8
	}
	for i < len(src) && src[i] < utf8.RuneSelf {
		i++
	}
	if i == len(src) {
		return true
	}
	src = src[i:]
	if len(src) < 32 {
		return utf8.Valid(src)
	}

	original := src
	r, size := utf8.DecodeRune(src)
	if r == utf8.RuneError && size == 1 {
		return false
	}
	src = src[size:]

	if len(original) >= wideThreshold && archsimd.X86.AVX2() {
		return validWide(original, src, size)
	}
	return validNarrow(original, src)
}

func validNarrow(original, src []byte) bool {
	const width = 16

	firstHighTable := archsimd.LoadUint8x16Array(&utf8FirstHigh)
	firstLowTable := archsimd.LoadUint8x16Array(&utf8FirstLow)
	secondHighTable := archsimd.LoadUint8x16Array(&utf8SecondHigh)
	lowNibble := archsimd.BroadcastUint8x16(0x0f)
	continuationBit := archsimd.BroadcastUint8x16(0x80)
	thirdThreshold := archsimd.BroadcastUint8x16(0xdf)
	fourthThreshold := archsimd.BroadcastUint8x16(0xef)
	zero := archsimd.BroadcastUint8x16(0)

	var previous archsimd.Uint8x16
	for len(src) >= 2*width {
		input0 := archsimd.LoadUint8x16(src)
		previous01 := input0.ConcatShiftBytesRight(previous, 15)
		previous02 := input0.ConcatShiftBytesRight(previous, 14)
		previous03 := input0.ConcatShiftBytesRight(previous, 13)

		previous01High := previous01.AsUint16x8().ShiftAllRight(4).AsUint8x16().And(lowNibble)
		input0High := input0.AsUint16x8().ShiftAllRight(4).AsUint8x16().And(lowNibble)
		special0 := firstHighTable.PermuteOrZero(previous01High.AsInt8x16()).
			And(firstLowTable.PermuteOrZero(previous01.And(lowNibble).AsInt8x16())).
			And(secondHighTable.PermuteOrZero(input0High.AsInt8x16()))

		mustContinue0 := previous02.SubSaturated(thirdThreshold).
			Or(previous03.SubSaturated(fourthThreshold)).
			Greater(zero)
		required0 := continuationBit.Masked(mustContinue0)
		if !required0.Xor(special0).IsZero() {
			return false
		}

		input1 := archsimd.LoadUint8x16(src[width:])
		previous11 := input1.ConcatShiftBytesRight(input0, 15)
		previous12 := input1.ConcatShiftBytesRight(input0, 14)
		previous13 := input1.ConcatShiftBytesRight(input0, 13)

		previous11High := previous11.AsUint16x8().ShiftAllRight(4).AsUint8x16().And(lowNibble)
		input1High := input1.AsUint16x8().ShiftAllRight(4).AsUint8x16().And(lowNibble)
		special1 := firstHighTable.PermuteOrZero(previous11High.AsInt8x16()).
			And(firstLowTable.PermuteOrZero(previous11.And(lowNibble).AsInt8x16())).
			And(secondHighTable.PermuteOrZero(input1High.AsInt8x16()))

		mustContinue1 := previous12.SubSaturated(thirdThreshold).
			Or(previous13.SubSaturated(fourthThreshold)).
			Greater(zero)
		required1 := continuationBit.Masked(mustContinue1)
		if !required1.Xor(special1).IsZero() {
			return false
		}

		previous = input1
		src = src[2*width:]
	}

	for len(src) >= width {
		input := archsimd.LoadUint8x16(src)
		previous1 := input.ConcatShiftBytesRight(previous, 15)
		previous2 := input.ConcatShiftBytesRight(previous, 14)
		previous3 := input.ConcatShiftBytesRight(previous, 13)

		previous1High := previous1.AsUint16x8().ShiftAllRight(4).AsUint8x16().And(lowNibble)
		inputHigh := input.AsUint16x8().ShiftAllRight(4).AsUint8x16().And(lowNibble)
		special := firstHighTable.PermuteOrZero(previous1High.AsInt8x16()).
			And(firstLowTable.PermuteOrZero(previous1.And(lowNibble).AsInt8x16())).
			And(secondHighTable.PermuteOrZero(inputHigh.AsInt8x16()))

		mustContinue := previous2.SubSaturated(thirdThreshold).
			Or(previous3.SubSaturated(fourthThreshold)).
			Greater(zero)
		required := continuationBit.Masked(mustContinue)
		if !required.Xor(special).IsZero() {
			return false
		}

		previous = input
		src = src[width:]
	}

	if len(src) != 0 {
		input, _ := archsimd.LoadUint8x16Part(src)
		previous1 := input.ConcatShiftBytesRight(previous, 15)
		previous2 := input.ConcatShiftBytesRight(previous, 14)
		previous3 := input.ConcatShiftBytesRight(previous, 13)

		previous1High := previous1.AsUint16x8().ShiftAllRight(4).AsUint8x16().And(lowNibble)
		inputHigh := input.AsUint16x8().ShiftAllRight(4).AsUint8x16().And(lowNibble)
		special := firstHighTable.PermuteOrZero(previous1High.AsInt8x16()).
			And(firstLowTable.PermuteOrZero(previous1.And(lowNibble).AsInt8x16())).
			And(secondHighTable.PermuteOrZero(inputHigh.AsInt8x16()))

		mustContinue := previous2.SubSaturated(thirdThreshold).
			Or(previous3.SubSaturated(fourthThreshold)).
			Greater(zero)
		required := continuationBit.Masked(mustContinue)
		if !required.Xor(special).IsZero() {
			return false
		}
	}

	last := len(original) - 1
	for last > 0 && original[last]&0xc0 == 0x80 {
		last--
	}
	return utf8.Valid(original[last:])
}

var (
	utf8FirstHighAVX2 = [32]byte{
		0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
		0x80, 0x80, 0x80, 0x80, 0x21, 0x01, 0x15, 0x49,
		0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
		0x80, 0x80, 0x80, 0x80, 0x21, 0x01, 0x15, 0x49,
	}
	utf8FirstLowAVX2 = [32]byte{
		0xe7, 0xa3, 0x83, 0x83, 0x8b, 0xcb, 0xcb, 0xcb,
		0xcb, 0xcb, 0xcb, 0xcb, 0xcb, 0xdb, 0xcb, 0xcb,
		0xe7, 0xa3, 0x83, 0x83, 0x8b, 0xcb, 0xcb, 0xcb,
		0xcb, 0xcb, 0xcb, 0xcb, 0xcb, 0xdb, 0xcb, 0xcb,
	}
	utf8SecondHighAVX2 = [32]byte{
		0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
		0xe6, 0xae, 0xba, 0xba, 0x01, 0x01, 0x01, 0x01,
		0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
		0xe6, 0xae, 0xba, 0xba, 0x01, 0x01, 0x01, 0x01,
	}
)

// validWide is the 256-bit AVX2 kernel; src starts processed bytes into original.
func validWide(original, src []byte, processed int) bool {
	const width = 32

	firstHighTable := archsimd.LoadUint8x32Array(&utf8FirstHighAVX2)
	firstLowTable := archsimd.LoadUint8x32Array(&utf8FirstLowAVX2)
	secondHighTable := archsimd.LoadUint8x32Array(&utf8SecondHighAVX2)
	lowNibble := archsimd.BroadcastUint8x32(0x0f)
	continuationBit := archsimd.BroadcastUint8x32(0x80)
	thirdThreshold := archsimd.BroadcastUint8x32(0xdf)
	fourthThreshold := archsimd.BroadcastUint8x32(0xef)
	zero := archsimd.BroadcastUint8x32(0)

	var previous archsimd.Uint8x32
	for len(src) >= width {
		input := archsimd.LoadUint8x32(src)

		// AVX2 byte shifts operate independently on 128-bit lanes. Arrange the
		// preceding lane beside each current lane so UTF-8 sequences spanning
		// either the vector or lane boundary retain their prior three bytes.
		prior := input.SetLo(previous.GetHi()).SetHi(input.GetLo())
		previous1 := input.ConcatShiftBytesRightGrouped(prior, 15)
		previous2 := input.ConcatShiftBytesRightGrouped(prior, 14)
		previous3 := input.ConcatShiftBytesRightGrouped(prior, 13)

		previous1High := previous1.AsUint16x16().ShiftAllRight(4).AsUint8x32().And(lowNibble)
		inputHigh := input.AsUint16x16().ShiftAllRight(4).AsUint8x32().And(lowNibble)
		special := firstHighTable.PermuteOrZeroGrouped(previous1High.AsInt8x32()).
			And(firstLowTable.PermuteOrZeroGrouped(previous1.And(lowNibble).AsInt8x32())).
			And(secondHighTable.PermuteOrZeroGrouped(inputHigh.AsInt8x32()))

		mustContinue := previous2.SubSaturated(thirdThreshold).
			Or(previous3.SubSaturated(fourthThreshold)).
			Greater(zero)
		required := continuationBit.Masked(mustContinue)
		if !required.Xor(special).IsZero() {
			archsimd.ClearAVXUpperBits()
			return false
		}

		previous = input
		src = src[width:]
		processed += width
	}

	archsimd.ClearAVXUpperBits()

	tailStart := processed - 1
	for tailStart > 0 && original[tailStart]&0xc0 == 0x80 {
		tailStart--
	}
	return utf8.Valid(original[tailStart:])
}
