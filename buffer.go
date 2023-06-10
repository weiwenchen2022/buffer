// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package buffer

// Simple byte buffer for marshaling data.

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"unicode/utf8"
	"unsafe"
)

// A Buffer is a variable-sized buffer of bytes with Read and Write methods.
// The zero value for Buffer is an empty buffer ready to use.
// Do not copy a Buffer after first use.
type Buffer struct {
	addr     *Buffer // of receiver, to detect copies by value
	buf      []byte  // contents are the bytes buf[off : len(buf)]
	off      int     // read at &buf[off], write at &buf[len(buf)]
	lastRead readOp  // last read operation, so that Unread* can work correctly.
}

// noescape hides a pointer from escape analysis. It is the identity function
// but escape analysis doesn't think the output depends on the input.
// noescape is inlined and currently compiles down to zero instructions.
// USE CAREFULLY!
// This was copied from the runtime; see issues 23382 and 7921.
//
//go:nosplit
//go:nocheckptr
func noescape(p unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(uintptr(p))
}

func (b *Buffer) copyCheck() {
	if b.addr == nil {
		// This hack works around a failing of Go's escape analysis
		// that was causing b to escape and be heap allocated.
		// See issue 23382.
		// TODO: once issue 7921 is fixed, this should be reverted to
		// just "b.addr = b".
		b.addr = (*Buffer)(noescape(unsafe.Pointer(b)))
	} else if b != b.addr {
		panic("buffer: illegal use of non-zero Buffer copied by value")
	}
}

// The readOp constants describe the last action performed on
// the buffer, so that UnreadRune and UnreadByte can check for
// invalid usage. opReadRuneX constants are chosen such that
// converted to int they correspond to the rune size that was read.
type readOp int8

// Don't use iota for these, as the values need to correspond with the
// names and comments, which is easier to see when being explicit.
const (
	opRead      readOp = -1 // Any other read operation.
	opInvalid   readOp = 0  // Non-read operation.
	opReadRune1 readOp = 1  // Read rune of size 1.
	opReadRune2 readOp = 2  // Read rune of size 2.
	opReadRune3 readOp = 3  // Read rune of size 3.
	opReadRune4 readOp = 4  // Read rune of size 4.
)

// Bytes returns a slice of length b.Len() holding the unread portion of the buffer.
// The slice is valid for use only until the next buffer modification (that is,
// only until the next call to a method like Read, Write, Reset, or Truncate).
// The slice aliases the buffer content at least until the next buffer modification,
// so immediate changes to the slice will affect the result of future reads.
func (b *Buffer) Bytes() []byte { return b.buf[b.off:len(b.buf):len(b.buf)] }

// String returns the contents of the unread portion of the buffer
// as a string. If the Buffer is a nil pointer, it returns "<nil>".
//
// To build strings more efficiently, see the builder.Builder type.
func (b *Buffer) String() string {
	if b == nil {
		// Special case, useful in debugging.
		return "<nil>"
	}

	return string(b.buf[b.off:])
}

// empty reports whether the unread portion of the buffer is empty.
func (b *Buffer) empty() bool { return len(b.buf) == b.off }

// Len returns the number of bytes of the unread portion of the buffer;
// b.Len() == len(b.Bytes()).
func (b *Buffer) Len() int { return len(b.buf) - b.off }

// Cap returns the capacity of the buffer's underlying byte slice, that is, the
// total space allocated for the buffer's data.
func (b *Buffer) Cap() int { return cap(b.buf) }

// Truncate discards all but the first n unread bytes from the buffer
// but continues to use the same allocated storage.
// It panics if n is negative or greater than the length of the buffer.
func (b *Buffer) Truncate(n int) {
	if n == 0 {
		b.Reset()
		return
	}

	b.lastRead = opInvalid
	if n < 0 || n > b.Len() {
		panic("buffer.Buffer: truncation out of range")
	}
	copy(b.buf, b.buf[b.off:b.off+n])
	b.buf = b.buf[:n]
	b.off = 0
}

// Reset resets the buffer to be empty,
// but it retains the underlying storage for use by future writes.
// Reset is the same as Truncate(0).
func (b *Buffer) Reset() {
	b.buf = b.buf[:0]
	b.off = 0
	b.lastRead = opInvalid
}

// tryGrowByReslice is a inlineable version of grow for the fast-case where the
// internal buffer only needs to be resliced.
// It returns the index where bytes should be written and whether it succeeded.
func (b *Buffer) tryGrowByReslice(n int) (int, bool) {
	if l := len(b.buf); n <= cap(b.buf)-l {
		b.buf = b.buf[:l+n]
		return l, true
	}
	return 0, false
}

// ErrTooLarge is passed to panic if memory cannot be allocated to store data in a buffer.
var ErrTooLarge = errors.New("buffer.Buffer: too large")

// growSlice grows b by n, preserving the original content of b.
// If the allocation fails, it panics with ErrTooLarge.
func growSlice(b []byte, n int) []byte {
	didPanic := true
	defer func() {
		if didPanic {
			panic(ErrTooLarge)
		}
	}()

	// TODO(http://golang.org/issue/51462): We should rely on the append-make
	// pattern so that the compiler can call runtime.growslice. For example:
	//	return append(b, make([]byte, n)...)
	// This avoids unnecessary zero-ing of the first len(b) bytes of the
	// allocated slice, but this pattern causes b to escape onto the heap.
	//
	// Instead use the append-make pattern with a nil slice to ensure that
	// we allocate buffers rounded up to the closest size class.
	c := len(b) + n // ensure enough space for n elements
	if c < 2*cap(b) {
		// The growth rate has historically always been 2x. In the future,
		// we could rely purely on append to determine the growth rate.
		c = 2 * cap(b)
	}
	b2 := make([]byte, c)
	copy(b2, b)
	// b2 := append([]byte(nil), make([]byte, c)...)
	// copy(b2, b)
	didPanic = false
	return b2[:len(b)]
}

// smallBufferSize is an initial allocation minimal capacity.
const smallBufferSize = 64

const maxInt = int(^uint(0) >> 1)

// grow grows the buffer to guarantee space for n more bytes.
// It returns the index where bytes should be written.
// If the buffer can't grow it will panic with ErrTooLarge.
func (b *Buffer) grow(n int) int {
	m := b.Len()

	// If buffer is empty, reset to recover space.
	if m == 0 && b.off != 0 {
		b.Reset()
	}

	// Try to grow by means of a reslice.
	if i, ok := b.tryGrowByReslice(n); ok {
		return i
	}

	if b.buf == nil && n <= smallBufferSize {
		b.buf = make([]byte, n, smallBufferSize)
		return 0
	}

	c := cap(b.buf)
	if n <= c/2-m {
		// We can slide things down instead of allocating a new
		// slice. We only need m+n <= c to slide, but
		// we instead let capacity get twice as large so we
		// don't spend all our time copying.
		copy(b.buf, b.buf[b.off:])
	} else if c > maxInt-c-n {
		panic(ErrTooLarge)
	} else {
		// Add b.off to account for b.buf[:b.off] being sliced off the front.
		b.buf = growSlice(b.buf[b.off:], b.off+n)
	}

	// Restore b.off and len(b.buf).
	b.off = 0
	b.buf = b.buf[:m+n]
	return m
}

// Grow grows the buffer's capacity, if necessary, to guarantee space for
// another n bytes. After Grow(n), at least n bytes can be written to the
// buffer without another allocation.
// If n is negative, Grow will panic.
// If the buffer can't grow it will panic with ErrTooLarge.
func (b *Buffer) Grow(n int) {
	b.copyCheck()
	if n < 0 {
		panic("buffer.Buffer.Grow: negative count")
	}

	m := b.grow(n)
	b.buf = b.buf[:m]
}

// Clip removes unused capacity from the buffer.
func (b *Buffer) Clip() {
	b.copyCheck()
	b.buf = b.buf[: b.off+len(b.buf) : b.off+len(b.buf)]
}

// Write appends the contents of p to the buffer, growing the buffer as
// needed. The return value n is the length of p; err is always nil. If the
// buffer becomes too large, Write will panic with ErrTooLarge.
func (b *Buffer) Write(p []byte) (n int, err error) {
	b.copyCheck()
	b.lastRead = opInvalid
	m, ok := b.tryGrowByReslice(len(p))
	if !ok {
		m = b.grow(len(p))
	}
	return copy(b.buf[m:], p), nil
}

// WriteString appends the contents of s to the buffer, growing the buffer as
// needed. The return value n is the length of s; err is always nil. If the
// buffer becomes too large, WriteString will panic with ErrTooLarge.
func (b *Buffer) WriteString(s string) (n int, err error) {
	b.copyCheck()
	b.lastRead = opInvalid
	m, ok := b.tryGrowByReslice(len(s))
	if !ok {
		m = b.grow(len(s))
	}
	return copy(b.buf[m:], s), nil
}

// MinRead is the minimum slice size passed to a Read call by
// Buffer.ReadFrom. As long as the Buffer has at least MinRead bytes beyond
// what is required to hold the contents of r, ReadFrom will not grow the
// underlying buffer.
const MinRead = 512

var errNegativeRead = errors.New("buffer.Buffer: reader returned negative count from Read")

// ReadFrom reads data from r until EOF and appends it to the buffer, growing
// the buffer as needed. The return value n is the number of bytes read. Any
// error except io.EOF encountered during the read is also returned. If the
// buffer becomes too large, ReadFrom will panic with ErrTooLarge.
func (b *Buffer) ReadFrom(r io.Reader) (n int64, err error) {
	b.copyCheck()
	b.lastRead = opInvalid

	for {
		i := b.grow(MinRead)
		b.buf = b.buf[:i]

		m, e := r.Read(b.buf[i:cap(b.buf)])
		if m < 0 {
			panic(errNegativeRead)
		}

		b.buf = b.buf[:i+m]
		n += int64(m)
		if e == io.EOF {
			return n, nil // e is EOF, so return nil explicitly
		}
		if e != nil {
			return n, e
		}
	}
}

// WriteTo writes data to w until the buffer is drained or an error occurs.
// The return value n is the number of bytes written; it always fits into an
// int, but it is int64 to match the io.WriterTo interface. Any error
// encountered during the write is also returned.
func (b *Buffer) WriteTo(w io.Writer) (n int64, err error) {
	b.lastRead = opInvalid

	if nBytes := b.Len(); nBytes > 0 {
		m, e := w.Write(b.buf[b.off:])
		if m > nBytes {
			panic("buffer.Buffer.WriteTo: invalid Write count")
		}

		b.off += m
		n = int64(m)
		if e != nil {
			return n, e
		}

		// all bytes should have been written, by definition of
		// Write method in io.Writer
		if nBytes != m {
			return n, io.ErrShortWrite
		}
	}

	// Buffer is now empty; reset.
	b.Reset()
	return n, nil
}

// WriteByte appends the byte c to the buffer, growing the buffer as needed.
// The returned error is always nil, but is included to match bufio.Writer's
// WriteByte. If the buffer becomes too large, WriteByte will panic with
// ErrTooLarge.
func (b *Buffer) WriteByte(c byte) error {
	b.copyCheck()
	b.lastRead = opInvalid
	m, ok := b.tryGrowByReslice(1)
	if !ok {
		m = b.grow(1)
	}
	b.buf[m] = c
	return nil
}

// WriteRune appends the UTF-8 encoding of Unicode code point r to the
// buffer, returning its length and an error, which is always nil but is
// included to match bufio.Writer's WriteRune. The buffer is grown as needed;
// if it becomes too large, WriteRune will panic with ErrTooLarge.
func (b *Buffer) WriteRune(r rune) (n int, err error) {
	// Compare as uint32 to correctly handle negative runes.
	if uint32(r) < utf8.RuneSelf {
		_ = b.WriteByte(byte(r))
		return 1, nil
	}

	b.copyCheck()
	b.lastRead = opInvalid
	m, ok := b.tryGrowByReslice(utf8.UTFMax)
	if !ok {
		m = b.grow(utf8.UTFMax)
	}
	b.buf = utf8.AppendRune(b.buf[:m], r)
	return len(b.buf) - m, nil
}

// WriteBool appends "true" or "false", according to the value of b,
// to the buffer and returns written length and an error, which is always nil.
// The buffer is grown as needed;
// if it becomes too large, WriteBool will panic with ErrTooLarge.
func (b *Buffer) WriteBool(v bool) (int, error) {
	b.copyCheck()
	b.lastRead = opInvalid
	m, ok := b.tryGrowByReslice(5)
	if !ok {
		m = b.grow(5)
	}
	b.buf = strconv.AppendBool(b.buf[:m], v)
	return len(b.buf) - m, nil
}

// WriteInt appends the string form of the integer i,
// as generated by FormatInt, to the buffer and returns written length and an error,
// which is always nil. The buffer is grown as needed;
// if it becomes too large, WriteFloat will panic with ErrTooLarge.
func (b *Buffer) WriteInt(i int64, base int) (int, error) {
	b.copyCheck()
	b.lastRead = opInvalid
	m := len(b.buf)

	didPanic := true
	defer func() {
		if didPanic {
			panic(ErrTooLarge)
		}
	}()

	b.buf = strconv.AppendInt(b.buf[:m], i, base)
	didPanic = false
	return len(b.buf) - m, nil
}

// WriteUint appends the string form of the unsigned integer i,
// as generated by FormatUint, to the buffer and returns written length and an error,
// which is always nil. The buffer is grown as needed;
// if it becomes too large, WriteFloat will panic with ErrTooLarge.
func (b *Buffer) WriteUint(i uint64, base int) (int, error) {
	b.copyCheck()
	b.lastRead = opInvalid
	m := len(b.buf)

	didPanic := true
	defer func() {
		if didPanic {
			panic(ErrTooLarge)
		}
	}()

	b.buf = strconv.AppendUint(b.buf[:m], i, base)
	didPanic = false
	return len(b.buf) - m, nil
}

// WriteFloat appends the string form of the floating-point number f,
// as generated by FormatFloat, to the buffer and returns written length and an error,
// which is always nil. The buffer is grown as needed;
// if it becomes too large, WriteFloat will panic with ErrTooLarge.
func (b *Buffer) WriteFloat(f float64, fmt byte, prec, bitSize int) (int, error) {
	b.copyCheck()
	b.lastRead = opInvalid
	m := len(b.buf)

	didPanic := true
	defer func() {
		if didPanic {
			panic(ErrTooLarge)
		}
	}()
	b.buf = strconv.AppendFloat(b.buf[:m], f, fmt, prec, bitSize)
	didPanic = false
	return len(b.buf) - m, nil
}

func (b *Buffer) writeQuote(s string, quoteFn func([]byte, string) []byte) (int, error) {
	b.copyCheck()
	b.lastRead = opInvalid
	m := len(b.buf)

	didPanic := true
	defer func() {
		if didPanic {
			panic(ErrTooLarge)
		}
	}()

	b.buf = quoteFn(b.buf[:m], s)
	didPanic = false
	return len(b.buf) - m, nil
}

// WriteQuote appends a double-quoted Go string literal representing s,
// as generated by Quote, to the buffer and returns written length and an error,
// which is always nil. The buffer is grown as needed;
// if it becomes too large, WriteFloat will panic with ErrTooLarge.
func (b *Buffer) WriteQuote(s string) (int, error) {
	return b.writeQuote(s, strconv.AppendQuote)
}

// WriteQuoteToASCII appends a double-quoted Go string literal representing s,
// as generated by QuoteToASCII, to the buffer and returns written length and an error,
// which is always nil. The buffer is grown as needed;
// if it becomes too large, WriteFloat will panic with ErrTooLarge.
func (b *Buffer) WriteQuoteToASCII(s string) (int, error) {
	return b.writeQuote(s, strconv.AppendQuoteToASCII)
}

// WriteQuoteToGraphic appends a double-quoted Go string literal representing s,
// as generated by QuoteToGraphic, to the buffer and returns written length and an error,
// which is always nil. The buffer is grown as needed;
// if it becomes too large, WriteFloat will panic with ErrTooLarge.
func (b *Buffer) WriteQuoteToGraphic(s string) (int, error) {
	return b.writeQuote(s, strconv.AppendQuoteToGraphic)
}

func (b *Buffer) writeQuoteRune(r rune, quoteRuneFn func([]byte, rune) []byte) (int, error) {
	b.copyCheck()
	b.lastRead = opInvalid
	m := len(b.buf)

	didPanic := true
	defer func() {
		if didPanic {
			panic(ErrTooLarge)
		}
	}()

	b.buf = quoteRuneFn(b.buf[:m], r)
	didPanic = false
	return len(b.buf) - m, nil
}

// WriteQuoteRune appends a single-quoted Go character literal representing the rune,
// as generated by QuoteRune, to dst and returns written length and an error,
// which is always nil. The buffer is grown as needed;
// if it becomes too large, WriteFloat will panic with ErrTooLarge.
func (b *Buffer) WriteQuoteRune(r rune) (int, error) {
	return b.writeQuoteRune(r, strconv.AppendQuoteRune)
}

// WriteQuoteRuneToASCII appends a single-quoted Go character literal representing the rune,
// as generated by QuoteRuneToASCII, to the buffer and returns written length and an error,
// which is always nil. The buffer is grown as needed;
// if it becomes too large, WriteFloat will panic with ErrTooLarge.
func (b *Buffer) WriteQuoteRuneToASCII(r rune) (int, error) {
	return b.writeQuoteRune(r, strconv.AppendQuoteRuneToASCII)
}

// WriteQuoteRuneToGraphic appends a single-quoted Go character literal representing the rune,
// as generated by QuoteRuneToGraphic, to dst and returns  written length and an error,
// which is always nil. The buffer is grown as needed;
// if it becomes too large, WriteFloat will panic with ErrTooLarge.
func (b *Buffer) WriteQuoteRuneToGraphic(r rune) (int, error) {
	return b.writeQuoteRune(r, strconv.AppendQuoteRuneToGraphic)
}

// Read reads the next len(p) bytes from the buffer or until the buffer
// is drained. The return value n is the number of bytes read. If the
// buffer has no data to return, err is io.EOF (unless len(p) is zero);
// otherwise it is nil.
func (b *Buffer) Read(p []byte) (n int, err error) {
	b.lastRead = opInvalid

	if b.empty() {
		// Buffer is empty, reset to recover space.
		b.Reset()

		if len(p) == 0 {
			return 0, nil
		}
		return 0, io.EOF
	}

	n = copy(p, b.buf[b.off:])
	b.off += n
	if n > 0 {
		b.lastRead = opRead
	}
	return n, nil
}

// Next returns a slice containing the next n bytes from the buffer,
// advancing the buffer as if the bytes had been returned by Read.
// If there are fewer than n bytes in the buffer, Next returns the entire buffer.
// The slice is only valid until the next call to a read or write method.
func (b *Buffer) Next(n int) []byte {
	b.lastRead = opInvalid

	if l := b.Len(); l < n {
		n = l
	}

	data := b.buf[b.off : b.off+n : b.off+n]
	b.off += n
	if n > 0 {
		b.lastRead = opRead
	}
	return data
}

// ReadByte reads and returns the next byte from the buffer.
// If no byte is available, it returns error io.EOF.
func (b *Buffer) ReadByte() (byte, error) {
	if b.empty() {
		// Buffer is empty, reset to recover space.
		b.Reset()
		return 0, io.EOF
	}

	c := b.buf[b.off]
	b.off++
	b.lastRead = opRead
	return c, nil
}

// ReadRune reads and returns the next UTF-8-encoded
// Unicode code point from the buffer.
// If no bytes are available, the error returned is io.EOF.
// If the bytes are an erroneous UTF-8 encoding, it
// consumes one byte and returns U+FFFD, 1.
func (b *Buffer) ReadRune() (r rune, size int, err error) {
	if b.empty() {
		// Buffer is empty, reset to recover space.
		b.Reset()
		return 0, 0, io.EOF
	}

	c := b.buf[b.off]
	if c < utf8.RuneSelf {
		b.off++
		b.lastRead = opReadRune1
		return rune(c), 1, nil
	}

	r, size = utf8.DecodeRune(b.buf[b.off:])
	b.off += size
	b.lastRead = readOp(size)
	return r, size, nil
}

var errUnreadRune = errors.New("buffer.Buffer: UnreadRune: previous operation was not a successful ReadRune")

// UnreadRune unreads the last rune returned by ReadRune.
// If the most recent read or write operation on the buffer was
// not a successful ReadRune, UnreadRune returns an error. (In this regard
// it is stricter than UnreadByte, which will unread the last byte
// from any read operation.)
func (b *Buffer) UnreadRune() error {
	switch b.lastRead {
	default:
		return errUnreadRune
	case opReadRune1, opReadRune2, opReadRune3, opReadRune4:
	}

	b.off -= int(b.lastRead)
	b.lastRead = opInvalid
	return nil
}

var errUnreadByte = errors.New("buffer.Buffer: UnreadByte: previous operation was not a successful read")

// UnreadByte unreads the last byte returned by the most recent successful
// read operation that read at least one byte. If a write has happened since
// the last read, if the last read returned an error, or if the read read zero
// bytes, UnreadByte returns an error.
func (b *Buffer) UnreadByte() error {
	if b.lastRead == opInvalid {
		return errUnreadByte
	}

	b.off--
	b.lastRead = opInvalid
	return nil
}

// ReadBytes reads until the first occurrence of delim in the input,
// returning a slice containing the data up to and including the delimiter.
// If ReadBytes encounters an error before finding a delimiter,
// it returns the data read before the error and the error itself (often io.EOF).
// ReadBytes returns err != nil if and only if the returned data does not end in
// delim.
func (b *Buffer) ReadBytes(delim byte) (line []byte, err error) {
	slice, err := b.readSlice(delim)
	// return a copy of slice. The buffer's backing array may
	// be overwritten by later calls.
	line = append(line, slice...)
	return line, err
}

// readSlice is like ReadBytes but returns a reference to internal buffer data.
func (b *Buffer) readSlice(delim byte) (line []byte, err error) {
	i := bytes.IndexByte(b.buf[b.off:], delim)
	end := b.off + i + 1
	if i < 0 {
		end = len(b.buf)
		err = io.EOF
	}

	line = b.buf[b.off:end]
	b.off = end
	b.lastRead = opRead
	return line, err
}

// ReadString reads until the first occurrence of delim in the input,
// returning a string containing the data up to and including the delimiter.
// If ReadString encounters an error before finding a delimiter,
// it returns the data read before the error and the error itself (often io.EOF).
// ReadString returns err != nil if and only if the returned data does not end
// in delim.
func (b *Buffer) ReadString(delim byte) (line string, err error) {
	slice, err := b.readSlice(delim)
	return string(slice), err
}

// NewBuffer creates and initializes a new Buffer using buf as its
// initial contents. The new Buffer takes ownership of buf, and the
// caller should not use buf after this call. NewBuffer is intended to
// prepare a Buffer to read existing data. It can also be used to set
// the initial size of the internal buffer for writing. To do that,
// buf should have the desired capacity but a length of zero.
//
// In most cases, new(Buffer) (or just declaring a Buffer variable) is
// sufficient to initialize a Buffer.
func NewBuffer(buf []byte) *Buffer { return &Buffer{buf: buf} }

// NewBufferString creates and initializes a new Buffer using string s as its
// initial contents. It is intended to prepare a buffer to read an existing
// string.
//
// In most cases, new(Buffer) (or just declaring a Buffer variable) is
// sufficient to initialize a Buffer.
func NewBufferString(s string) *Buffer { return &Buffer{buf: []byte(s)} }