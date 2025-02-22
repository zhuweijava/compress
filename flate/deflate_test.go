// Copyright 2009 The Go Authors. All rights reserved.
// Copyright (c) 2015 Klaus Post
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flate

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type deflateTest struct {
	in    []byte
	level int
	out   []byte
}

type deflateInflateTest struct {
	in []byte
}

type reverseBitsTest struct {
	in       uint16
	bitCount uint8
	out      uint16
}

var deflateTests = []*deflateTest{
	{[]byte{}, 0, []byte{1, 0, 0, 255, 255}},
	{[]byte{0x11}, -1, []byte{18, 4, 4, 0, 0, 255, 255}},
	{[]byte{0x11}, DefaultCompression, []byte{18, 4, 4, 0, 0, 255, 255}},
	{[]byte{0x11}, 4, []byte{18, 4, 4, 0, 0, 255, 255}},

	{[]byte{0x11}, 0, []byte{0, 1, 0, 254, 255, 17, 1, 0, 0, 255, 255}},
	{[]byte{0x11, 0x12}, 0, []byte{0, 2, 0, 253, 255, 17, 18, 1, 0, 0, 255, 255}},
	{[]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11}, 0,
		[]byte{0, 8, 0, 247, 255, 17, 17, 17, 17, 17, 17, 17, 17, 1, 0, 0, 255, 255},
	},
	{[]byte{}, 1, []byte{1, 0, 0, 255, 255}},
	{[]byte{0x11}, 1, []byte{18, 4, 4, 0, 0, 255, 255}},
	{[]byte{0x11, 0x12}, 1, []byte{18, 20, 2, 4, 0, 0, 255, 255}},
	{[]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11}, 1, []byte{18, 132, 2, 64, 0, 0, 0, 255, 255}},
	{[]byte{}, 9, []byte{1, 0, 0, 255, 255}},
	{[]byte{0x11}, 9, []byte{18, 4, 4, 0, 0, 255, 255}},
	{[]byte{0x11, 0x12}, 9, []byte{18, 20, 2, 4, 0, 0, 255, 255}},
	{[]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11}, 9, []byte{18, 132, 2, 64, 0, 0, 0, 255, 255}},
}

var deflateInflateTests = []*deflateInflateTest{
	{[]byte{}},
	{[]byte{0x11}},
	{[]byte{0x11, 0x12}},
	{[]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11}},
	{[]byte{0x11, 0x10, 0x13, 0x41, 0x21, 0x21, 0x41, 0x13, 0x87, 0x78, 0x13}},
	{largeDataChunk()},
}

var reverseBitsTests = []*reverseBitsTest{
	{1, 1, 1},
	{1, 2, 2},
	{1, 3, 4},
	{1, 4, 8},
	{1, 5, 16},
	{17, 5, 17},
	{257, 9, 257},
	{29, 5, 23},
}

func largeDataChunk() []byte {
	result := make([]byte, 100000)
	for i := range result {
		result[i] = byte(i * i & 0xFF)
	}
	return result
}

func TestCRC(t *testing.T) {
	if !useSSE42 {
		t.Skip("Skipping CRC test, no SSE 4.2 available")
	}
	for _, x := range deflateTests {
		y := x.out
		if len(y) >= minMatchLength {
			t.Logf("In: %v, Out:0x%08x", y[0:minMatchLength], crc32sse(y[0:minMatchLength]))
		}
	}
}

func TestCRCBulk(t *testing.T) {
	if !useSSE42 {
		t.Skip("Skipping CRC test, no SSE 4.2 available")
	}
	for _, x := range deflateTests {
		y := x.out
		if len(y) >= minMatchLength {
			y = append(y, y...)
			for j := 4; j < len(y); j++ {
				y := y[:j]
				dst := make([]hash, len(y)-minMatchLength+1)
				for i := range dst {
					dst[i] = hash(i + 100)
				}
				crc32sseAll(y, dst)
				for i, got := range dst {
					expect := crc32sse(y[i:])
					if got != expect && got == hash(i)+100 {
						t.Errorf("Len:%d Index:%d, expected 0x%08x but not modified", len(y), i, expect)
					} else if got != expect {
						t.Errorf("Len:%d Index:%d, got 0x%08x expected:0x%08x", len(y), i, got, expect)
					} else {
						//t.Logf("Len:%d Index:%d OK", len(y), i)
					}
				}
			}
		}
	}
}

func TestCRCBulkOld(t *testing.T) {
	for _, x := range deflateTests {
		y := x.out
		if len(y) >= minMatchLength {
			y = append(y, y...)
			for j := 4; j < len(y); j++ {
				y := y[:j]
				dst := make([]hash, len(y)-minMatchLength+1)
				for i := range dst {
					dst[i] = hash(i + 100)
				}
				oldBulkHash(y, dst)
				for i, val := range dst {
					got := val & hashMask
					expect := oldHash(y[i:]) & hashMask
					if got != expect && got == hash(i)+100 {
						t.Errorf("Len:%d Index:%d, expected 0x%08x but not modified", len(y), i, expect)
					} else if got != expect {
						t.Errorf("Len:%d Index:%d, got 0x%08x expected:0x%08x", len(y), i, got, expect)
					} else {
						//t.Logf("Len:%d Index:%d OK (0x%08x)", len(y), i, got)
					}
				}
			}
		}
	}
}

func TestDeflate(t *testing.T) {
	for _, h := range deflateTests {
		var buf bytes.Buffer
		w, err := NewWriter(&buf, h.level)
		if err != nil {
			t.Errorf("NewWriter: %v", err)
			continue
		}
		w.Write(h.in)
		w.Close()
		if !bytes.Equal(buf.Bytes(), h.out) {
			t.Errorf("Deflate(%d, %x) = \n%#v, want \n%#v", h.level, h.in, buf.Bytes(), h.out)
		}
	}
}

// A sparseReader returns a stream consisting of 0s followed by 1<<16 1s.
// This tests missing hash references in a very large input.
type sparseReader struct {
	l   int64
	cur int64
}

func (r *sparseReader) Read(b []byte) (n int, err error) {
	if r.cur >= r.l {
		return 0, io.EOF
	}
	n = len(b)
	cur := r.cur + int64(n)
	if cur > r.l {
		n -= int(cur - r.l)
		cur = r.l
	}
	for i := range b[0:n] {
		if r.cur+int64(i) >= r.l-1<<16 {
			b[i] = 1
		} else {
			b[i] = 0
		}
	}
	r.cur = cur
	return
}

func TestVeryLongSparseChunk(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sparse chunk during short test")
	}
	w, err := NewWriter(ioutil.Discard, 1)
	if err != nil {
		t.Errorf("NewWriter: %v", err)
		return
	}
	if _, err = io.Copy(w, &sparseReader{l: 23E8}); err != nil {
		t.Errorf("Compress failed: %v", err)
		return
	}
}

type syncBuffer struct {
	buf    bytes.Buffer
	mu     sync.RWMutex
	closed bool
	ready  chan bool
}

func newSyncBuffer() *syncBuffer {
	return &syncBuffer{ready: make(chan bool, 1)}
}

func (b *syncBuffer) Read(p []byte) (n int, err error) {
	for {
		b.mu.RLock()
		n, err = b.buf.Read(p)
		b.mu.RUnlock()
		if n > 0 || b.closed {
			return
		}
		<-b.ready
	}
}

func (b *syncBuffer) signal() {
	select {
	case b.ready <- true:
	default:
	}
}

func (b *syncBuffer) Write(p []byte) (n int, err error) {
	n, err = b.buf.Write(p)
	b.signal()
	return
}

func (b *syncBuffer) WriteMode() {
	b.mu.Lock()
}

func (b *syncBuffer) ReadMode() {
	b.mu.Unlock()
	b.signal()
}

func (b *syncBuffer) Close() error {
	b.closed = true
	b.signal()
	return nil
}

func testSync(t *testing.T, level int, input []byte, name string) {
	if len(input) == 0 {
		return
	}

	t.Logf("--testSync %d, %d, %s", level, len(input), name)
	buf := newSyncBuffer()
	buf1 := new(bytes.Buffer)
	buf.WriteMode()
	w, err := NewWriter(io.MultiWriter(buf, buf1), level)
	if err != nil {
		t.Errorf("NewWriter: %v", err)
		return
	}
	r := NewReader(buf)

	// Write half the input and read back.
	for i := 0; i < 2; i++ {
		var lo, hi int
		if i == 0 {
			lo, hi = 0, (len(input)+1)/2
		} else {
			lo, hi = (len(input)+1)/2, len(input)
		}
		t.Logf("#%d: write %d-%d", i, lo, hi)
		if _, err := w.Write(input[lo:hi]); err != nil {
			t.Errorf("testSync: write: %v", err)
			return
		}
		if i == 0 {
			if err := w.Flush(); err != nil {
				t.Errorf("testSync: flush: %v", err)
				return
			}
		} else {
			if err := w.Close(); err != nil {
				t.Errorf("testSync: close: %v", err)
			}
		}
		buf.ReadMode()
		out := make([]byte, hi-lo+1)
		m, err := io.ReadAtLeast(r, out, hi-lo)
		t.Logf("#%d: read %d", i, m)
		if m != hi-lo || err != nil {
			t.Errorf("testSync/%d (%d, %d, %s): read %d: %d, %v (%d left)", i, level, len(input), name, hi-lo, m, err, buf.buf.Len())
			return
		}
		if !bytes.Equal(input[lo:hi], out[:hi-lo]) {
			t.Errorf("testSync/%d: read wrong bytes: %x vs %x", i, input[lo:hi], out[:hi-lo])
			return
		}
		// This test originally checked that after reading
		// the first half of the input, there was nothing left
		// in the read buffer (buf.buf.Len() != 0) but that is
		// not necessarily the case: the write Flush may emit
		// some extra framing bits that are not necessary
		// to process to obtain the first half of the uncompressed
		// data.  The test ran correctly most of the time, because
		// the background goroutine had usually read even
		// those extra bits by now, but it's not a useful thing to
		// check.
		buf.WriteMode()
	}
	buf.ReadMode()
	out := make([]byte, 10)
	if n, err := r.Read(out); n > 0 || err != io.EOF {
		t.Errorf("testSync (%d, %d, %s): final Read: %d, %v (hex: %x)", level, len(input), name, n, err, out[0:n])
	}
	if buf.buf.Len() != 0 {
		t.Errorf("testSync (%d, %d, %s): extra data at end", level, len(input), name)
	}
	r.Close()

	// stream should work for ordinary reader too
	r = NewReader(buf1)
	out, err = ioutil.ReadAll(r)
	if err != nil {
		t.Errorf("testSync: read: %s", err)
		return
	}
	r.Close()
	if !bytes.Equal(input, out) {
		t.Errorf("testSync: decompress(compress(data)) != data: level=%d input=%s", level, name)
	}
}

func testToFromWithLevelAndLimit(t *testing.T, level int, input []byte, name string, limit int) {
	var buffer bytes.Buffer
	w, err := NewWriter(&buffer, level)
	if err != nil {
		t.Errorf("NewWriter: %v", err)
		return
	}
	w.Write(input)
	w.Close()
	if limit > 0 && buffer.Len() > limit {
		t.Errorf("level: %d, len(compress(data)) = %d > limit = %d", level, buffer.Len(), limit)
		return
	}
	if limit > 0 {
		t.Logf("level: %d - Size:%.2f%%, %d b\n", level, float64(buffer.Len()*100)/float64(limit), buffer.Len())
	}
	r := NewReader(&buffer)
	out, err := ioutil.ReadAll(r)
	if err != nil {
		t.Errorf("read: %s", err)
		return
	}
	r.Close()
	if !bytes.Equal(input, out) {
		t.Errorf("decompress(compress(data)) != data: level=%d input=%s", level, name)
		return
	}
	testSync(t, level, input, name)
}

func testToFromWithLimit(t *testing.T, input []byte, name string, limit [11]int) {
	for i := 0; i < 10; i++ {
		testToFromWithLevelAndLimit(t, i, input, name, limit[i])
	}
	testToFromWithLevelAndLimit(t, -2, input, name, limit[10])
}

func TestDeflateInflate(t *testing.T) {
	for i, h := range deflateInflateTests {
		testToFromWithLimit(t, h.in, fmt.Sprintf("#%d", i), [11]int{})
	}
}

func TestReverseBits(t *testing.T) {
	for _, h := range reverseBitsTests {
		if v := reverseBits(h.in, h.bitCount); v != h.out {
			t.Errorf("reverseBits(%v,%v) = %v, want %v",
				h.in, h.bitCount, v, h.out)
		}
	}
}

type deflateInflateStringTest struct {
	filename string
	label    string
	limit    [11]int // Number 11 is ConstantCompression
}

var deflateInflateStringTests = []deflateInflateStringTest{
	{
		"../testdata/e.txt",
		"2.718281828...",
		[...]int{100018, 50650, 50960, 51150, 50930, 50790, 50790, 50790, 50790, 50790, 43683 + 100},
	},
	{
		"../testdata/Mark.Twain-Tom.Sawyer.txt",
		"Mark.Twain-Tom.Sawyer",
		[...]int{407330, 195000, 180361, 172974, 169160, 163476, 160936, 160506, 160295, 160295, 233460 + 100},
	},
}

func TestDeflateInflateString(t *testing.T) {
	for _, test := range deflateInflateStringTests {
		gold, err := ioutil.ReadFile(test.filename)
		if err != nil {
			t.Error(err)
		}
		// Remove returns that may be present on Windows
		neutral := strings.Map(func(r rune) rune {
			if r != '\r' {
				return r
			}
			return -1
		}, string(gold))

		testToFromWithLimit(t, []byte(neutral), test.label, test.limit)

		if testing.Short() {
			break
		}
	}
}

func TestReaderDict(t *testing.T) {
	const (
		dict = "hello world"
		text = "hello again world"
	)
	var b bytes.Buffer
	w, err := NewWriter(&b, 5)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	w.Write([]byte(dict))
	w.Flush()
	b.Reset()
	w.Write([]byte(text))
	w.Close()

	r := NewReaderDict(&b, []byte(dict))
	data, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello again world" {
		t.Fatalf("read returned %q want %q", string(data), text)
	}
}

func TestWriterDict(t *testing.T) {
	const (
		dict = "hello world"
		text = "hello again world"
	)
	var b bytes.Buffer
	w, err := NewWriter(&b, 5)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	w.Write([]byte(dict))
	w.Flush()
	b.Reset()
	w.Write([]byte(text))
	w.Close()

	var b1 bytes.Buffer
	w, _ = NewWriterDict(&b1, 5, []byte(dict))
	w.Write([]byte(text))
	w.Close()

	if !bytes.Equal(b1.Bytes(), b.Bytes()) {
		t.Fatalf("writer wrote %q want %q", b1.Bytes(), b.Bytes())
	}
}

// See http://code.google.com/p/go/issues/detail?id=2508
func TestRegression2508(t *testing.T) {
	if testing.Short() {
		t.Logf("test disabled with -short")
		return
	}
	w, err := NewWriter(ioutil.Discard, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	buf := make([]byte, 1024)
	for i := 0; i < 131072; i++ {
		if _, err := w.Write(buf); err != nil {
			t.Fatalf("writer failed: %v", err)
		}
	}
	w.Close()
}

func TestWriterReset(t *testing.T) {
	for level := -2; level <= 9; level++ {
		if level == -1 {
			level++
		}
		if testing.Short() && level > 1 {
			break
		}
		w, err := NewWriter(ioutil.Discard, level)
		if err != nil {
			t.Fatalf("NewWriter: %v", err)
		}
		buf := []byte("hello world")
		for i := 0; i < 1024; i++ {
			w.Write(buf)
		}
		w.Reset(ioutil.Discard)

		wref, err := NewWriter(ioutil.Discard, level)
		if err != nil {
			t.Fatalf("NewWriter: %v", err)
		}

		// DeepEqual doesn't compare functions.
		w.d.fill, wref.d.fill = nil, nil
		w.d.step, wref.d.step = nil, nil
		w.d.hasher, wref.d.hasher = nil, nil
		w.d.bulkHasher, wref.d.bulkHasher = nil, nil
		w.d.matcher, wref.d.matcher = nil, nil
		// hashMatch is always overwritten when used.
		copy(w.d.hashMatch[:], wref.d.hashMatch[:])
		if w.d.tokens.n != 0 {
			t.Errorf("level %d Writer not reset after Reset. %d tokens were present", level, w.d.tokens.n)
		}
		// As long as the length is 0, we don't care about the content.
		w.d.tokens = wref.d.tokens

		// We don't care if there are values in the window, as long as it is at d.index is 0
		w.d.window = wref.d.window
		if !reflect.DeepEqual(w, wref) {
			t.Errorf("level %d Writer not reset after Reset", level)
		}
	}
	testResetOutput(t, func(w io.Writer) (*Writer, error) { return NewWriter(w, NoCompression) })
	testResetOutput(t, func(w io.Writer) (*Writer, error) { return NewWriter(w, DefaultCompression) })
	testResetOutput(t, func(w io.Writer) (*Writer, error) { return NewWriter(w, BestCompression) })
	testResetOutput(t, func(w io.Writer) (*Writer, error) { return NewWriter(w, ConstantCompression) })
	dict := []byte("we are the world")
	testResetOutput(t, func(w io.Writer) (*Writer, error) { return NewWriterDict(w, NoCompression, dict) })
	testResetOutput(t, func(w io.Writer) (*Writer, error) { return NewWriterDict(w, DefaultCompression, dict) })
	testResetOutput(t, func(w io.Writer) (*Writer, error) { return NewWriterDict(w, BestCompression, dict) })
	testResetOutput(t, func(w io.Writer) (*Writer, error) { return NewWriterDict(w, ConstantCompression, dict) })
}

func testResetOutput(t *testing.T, newWriter func(w io.Writer) (*Writer, error)) {
	buf := new(bytes.Buffer)
	w, err := newWriter(buf)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	b := []byte("hello world")
	for i := 0; i < 1024; i++ {
		w.Write(b)
	}
	w.Close()
	out1 := buf.Bytes()

	buf2 := new(bytes.Buffer)
	w.Reset(buf2)
	for i := 0; i < 1024; i++ {
		w.Write(b)
	}
	w.Close()
	out2 := buf2.Bytes()

	if len(out1) != len(out2) {
		t.Errorf("got %d, expected %d bytes", len(out2), len(out1))
	}
	if bytes.Compare(out1, out2) != 0 {
		mm := 0
		for i, b := range out1[:len(out2)] {
			if b != out2[i] {
				t.Errorf("mismatch index %d: %02x, expected %02x", i, out2[i], b)
			}
			mm++
			if mm == 10 {
				t.Fatal("Stopping")
			}
		}
	}
	t.Logf("got %d bytes", len(out1))
}

// A writer that fails after N writes.
type errorWriter struct {
	N int
}

func (e *errorWriter) Write(b []byte) (int, error) {
	if e.N <= 0 {
		return 0, io.ErrClosedPipe
	}
	e.N--
	return len(b), nil
}

// Test if errors from the underlying writer is passed upwards.
func TestWriteError(t *testing.T) {
	buf := new(bytes.Buffer)
	for i := 0; i < 1024*1024; i++ {
		buf.WriteString(fmt.Sprintf("asdasfasf%d%dfghfgujyut%dyutyu\n", i, i, i))
	}
	in := buf.Bytes()
	for l := -2; l < 10; l++ {
		for fail := 1; fail <= 512; fail *= 2 {
			// Fail after 2 writes
			ew := &errorWriter{N: fail}
			w, err := NewWriter(ew, l)
			if err != nil {
				t.Errorf("NewWriter: level %d: %v", l, err)
			}
			n, err := io.Copy(w, bytes.NewBuffer(in))
			if err == nil {
				t.Errorf("Level %d: Expected an error, writer was %#v", l, ew)
			}
			n2, err := w.Write([]byte{1, 2, 2, 3, 4, 5})
			if n2 != 0 {
				t.Error("Level", l, "Expected 0 length write, got", n)
			}
			if err == nil {
				t.Error("Level", l, "Expected an error")
			}
			err = w.Flush()
			if err == nil {
				t.Error("Level", l, "Expected an error on close")
			}
			err = w.Close()
			if err == nil {
				t.Error("Level", l, "Expected an error on close")
			}

			w.Reset(ioutil.Discard)
			n2, err = w.Write([]byte{1, 2, 3, 4, 5, 6})
			if err != nil {
				t.Error("Level", l, "Got unexpected error after reset:", err)
			}
			if n2 == 0 {
				t.Error("Level", l, "Got 0 length write, expected > 0")
			}
			if testing.Short() {
				return
			}
		}
	}

}
