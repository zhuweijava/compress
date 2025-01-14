//+build !noasm
//+build !appengine

// Copyright 2015, Klaus Post, see LICENSE for details.

package flate

import (
	"github.com/klauspost/cpuid"
)

// crc32sse returns a hash for the first 4 bytes of the slice
// len(a) must be >= 4.
func crc32sse(a []byte) hash

// crc32sseAll calculates hashes for each 4-byte set in a.
// dst must be east len(a) - 4 in size.
// The size is not checked by the assembly.
func crc32sseAll(a []byte, dst []hash)

// matchLenSSE4 returns the number of matching bytes in a and b
// up to length 'max'. Both slices must be at least 'max'
// bytes in size.
// It uses the PCMPESTRI SSE 4.2 instruction.
func matchLenSSE4(a, b []byte, max int) int

// histogram accumulates a histogram of b in h.
// h must be at least 256 entries in length,
// and must be cleared before calling this function.
func histogram(b []byte, h []int32)

// Detect SSE 4.2 feature.
func init() {
	useSSE42 = cpuid.CPU.SSE42()
}
