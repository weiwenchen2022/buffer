// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package buffer_test

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"strconv"
	"testing"
	"time"
	"unicode/utf8"

	. "github.com/weiwenchen2022/buffer"
)

const N = 10000       // make this bigger for a larger (and slower) test
var testString string // test data for write tests
var testBytes []byte  // test data; same as testString but as a slice.

func init() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	testBytes = make([]byte, N)
	for i := range testBytes {
		testBytes[i] = 'a' + byte(r.Intn(26))
	}

	testString = string(testBytes)
}

// Verify that contents of buf match the string s.
func check(t testing.TB, testname string, buf *Buffer, s string) {
	t.Helper()

	bytes := buf.Bytes()
	str := buf.String()

	if len(bytes) != buf.Len() {
		t.Errorf("%s: buf.Len() == %d, len(buf.Bytes()) == %d", testname, buf.Len(), len(bytes))
	}

	if len(str) != buf.Len() {
		t.Errorf("%s: buf.Len() == %d, len(buf.String()) == %d", testname, buf.Len(), len(str))
	}

	if len(s) != buf.Len() {
		t.Errorf("%s: buf.Len() == %d, len(s) == %d", testname, buf.Len(), len(s))
	}

	if s != string(bytes) {
		t.Errorf("%s: string(buf.Bytes()) == %q, s == %q", testname, string(bytes), s)
	}
}

// Fill buf through n writes of string fus.
// The initial contents of buf corresponds to the string s;
// the result is the final contents of buf returned as a string.
func fillString(t testing.TB, testname string, buf *Buffer, s string, n int, fus string) string {
	t.Helper()

	check(t, testname+" (fill 1)", buf, s)
	for ; n > 0; n-- {
		m, err := buf.WriteString(fus)
		if err != nil {
			t.Errorf(testname+" (fill 2): err should always be nil, found err == %v", err)
		}
		if len(fus) != m {
			t.Errorf(testname+" (fill 3): m == %d, expected %d", m, len(fus))
		}

		s += fus
		check(t, testname+" (fill 4)", buf, s)
	}
	return buf.String()
}

// Fill buf through n writes of byte slice fub.
// The initial contents of buf corresponds to the string s;
// the result is the final contents of buf returned as a string.
func fillBytes(t testing.TB, testname string, buf *Buffer, s string, n int, fub []byte) string {
	t.Helper()

	check(t, testname+" (fill 1)", buf, s)
	for ; n > 0; n-- {
		m, err := buf.Write(fub)
		if err != nil {
			t.Errorf(testname+" (fill 2): err should always be nil, found err == %v", err)
		}
		if len(fub) != m {
			t.Errorf(testname+" (fill 3): m == %d, expected %d", m, len(fub))
		}

		s += string(fub)
		check(t, testname+" (fill 4)", buf, s)
	}
	return buf.String()
}

func TestNewBuffer(t *testing.T) {
	t.Parallel()
	buf := NewBuffer(testBytes)
	check(t, "NewBuffer", buf, testString)
}

func TestNewBufferString(t *testing.T) {
	t.Parallel()
	buf := NewBufferString(testString)
	check(t, "NewBufferString", buf, testString)
}

// Empty buf through repeated reads into fub.
// The initial contents of buf corresponds to the string s.
func empty(t testing.TB, testname string, buf *Buffer, s string, fub []byte) {
	t.Helper()

	check(t, testname+" (empty 1)", buf, s)
	for {
		n, err := buf.Read(fub)
		s = s[n:]
		check(t, testname+" (empty 2)", buf, s)

		if err == io.EOF {
			break
		}

		if err != nil {
			t.Errorf(testname+" (empty 3): err should always be nil, found err == %s", err)
		}
	}
	check(t, testname+" (empty 4)", buf, "")
}

func TestBasicOperations(t *testing.T) {
	t.Parallel()

	var buf Buffer
	for i := 0; i < 5; i++ {
		check(t, "TestBasicOperations (1)", &buf, "")

		buf.Reset()
		check(t, "TestBasicOperations (2)", &buf, "")

		buf.Truncate(0)
		check(t, "TestBasicOperations (3)", &buf, "")

		n, err := buf.Write(testBytes[:1])
		if err != nil || n != 1 {
			t.Errorf("Write: got (%d, %v), want (1, nil)", n, err)
		}
		check(t, "TestBasicOperations (4)", &buf, testString[:1])

		_ = buf.WriteByte(testBytes[1])
		check(t, "TestBasicOperations (5)", &buf, testString[:2])

		n, err = buf.Write(testBytes[2:26])
		if err != nil || n != 24 {
			t.Errorf("Write: got (%d, %v), want (24, nil)", n, err)
		}
		check(t, "TestBasicOperations (6)", &buf, testString[:26])

		buf.Truncate(26)
		check(t, "TestBasicOperations (7)", &buf, testString[:26])

		buf.Truncate(20)
		check(t, "TestBasicOperations (8)", &buf, testString[:20])

		empty(t, "TestBasicOperations (9)", &buf, testString[:20], make([]byte, 5))
		empty(t, "TestBasicOperations (10)", &buf, "", make([]byte, 100))

		_ = buf.WriteByte(testBytes[1])
		c, err := buf.ReadByte()
		if want := testBytes[1]; err != nil || want != c {
			t.Errorf("ReadByte: got (%q, %v), want (%q, nil)", c, err, want)
		}

		c, err = buf.ReadByte()
		if err != io.EOF {
			t.Errorf("ReadByte: got (%q, %v), want (0, %v)", c, err, io.EOF)
		}
	}
}

func TestLargeStringWrites(t *testing.T) {
	t.Parallel()

	limit := 30
	if testing.Short() {
		limit = 9
	}

	var buf Buffer
	for i := 3; i < limit; i += 3 {
		s := fillString(t, "TestLargeStringWrites (1)", &buf, "", 5, testString)
		empty(t, "TestLargeStringWrites (2)", &buf, s, make([]byte, len(testString)/i))
	}
	check(t, "TestLargeStringWrites (3)", &buf, "")
}

func TestLargeByteWrites(t *testing.T) {
	t.Parallel()

	limit := 30
	if testing.Short() {
		limit = 9
	}

	var buf Buffer
	for i := 3; i < limit; i += 3 {
		s := fillBytes(t, "TestLargeByteWrites (1)", &buf, "", 5, testBytes)
		empty(t, "TestLargeByteWrites (2)", &buf, s, make([]byte, len(testBytes)/i))
	}
	check(t, "TestLargeByteWrites (3)", &buf, "")
}

func TestLargeStringReads(t *testing.T) {
	t.Parallel()

	var buf Buffer
	for i := 3; i < 30; i += 3 {
		s := fillString(t, "TestLargeStringReads (1)", &buf, "", 5, testString[:len(testString)/i])
		empty(t, "TestLargeStringReads (2)", &buf, s, make([]byte, len(testString)))
	}
	check(t, "TestLargeStringReads (3)", &buf, "")
}

func TestLargeByteReads(t *testing.T) {
	t.Parallel()

	var buf Buffer
	for i := 3; i < 30; i += 3 {
		s := fillString(t, "TestLargeByteReads (1)", &buf, "", 5, string(testBytes[:len(testBytes)/i]))
		empty(t, "TestLargeByteReads (2)", &buf, s, make([]byte, len(testBytes)))
	}
	check(t, "TestLargeByteReads (3)", &buf, "")
}

func TestMixedReadsAndWrites(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var buf Buffer
	var s string
	for i := 0; i < 50; i++ {
		wlen := r.Intn(len(testString))
		if i%2 == 0 {
			s = fillString(t, "TestMixedReadsAndWrites (1)", &buf, s, 1, testString[:wlen])
		} else {
			s = fillBytes(t, "TestMixedReadsAndWrites (1)", &buf, s, 1, testBytes[:wlen])
		}

		rlen := r.Intn(len(testString))
		n, _ := buf.Read(make([]byte, rlen))
		s = s[n:]
	}
	empty(t, "TestMixedReadsAndWrites (2)", &buf, s, make([]byte, buf.Len()))
}

func TestCapWithPreallocatedSlice(t *testing.T) {
	t.Parallel()
	buf := NewBuffer(make([]byte, 10))
	if n := buf.Cap(); n != 10 {
		t.Errorf("expected 10, got %d", n)
	}
}

func TestCapWithSliceAndWrittenData(t *testing.T) {
	t.Parallel()
	buf := NewBuffer(make([]byte, 0, 10))
	_, _ = buf.Write([]byte("test"))
	if n := buf.Cap(); n != 10 {
		t.Errorf("expected 10, got %d", n)
	}
}

func TestNil(t *testing.T) {
	t.Parallel()
	var b *Buffer
	if b.String() != "<nil>" {
		t.Errorf("expected <nil>; got %q", b.String())
	}
}

func TestReadFrom(t *testing.T) {
	t.Parallel()
	var buf Buffer
	for i := 3; i < 30; i += 3 {
		s := fillBytes(t, "TestReadFrom (1)", &buf, "", 5, testBytes[:len(testBytes)/i])
		var b Buffer
		_, _ = b.ReadFrom(&buf)
		empty(t, "TestReadFrom (2)", &b, s, make([]byte, len(testBytes)))
	}
}

type panicReader struct{ panic bool }

func (r panicReader) Read(p []byte) (int, error) {
	if r.panic {
		panic(nil)
	}

	return 0, io.EOF
}

// Make sure that an empty Buffer remains empty when
// it is "grown" before a Read that panics
func TestReadFromPanicReader(t *testing.T) {
	t.Parallel()

	// First verify non-panic behaviour
	var buf Buffer
	n, err := buf.ReadFrom(panicReader{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("unexpected return from bytes.ReadFrom (1): got: %d, want 0", n)
	}
	check(t, "TestReadFromPanicReader (1)", &buf, "")

	// Confirm that when Reader panics, the empty buffer remains empty
	var buf2 Buffer
	defer func() {
		_ = recover()
		check(t, "TestReadFromPanicReader (2)", &buf2, "")
	}()
	_, _ = buf2.ReadFrom(panicReader{panic: true})
}

type negativeReader struct{}

func (r negativeReader) Read([]byte) (int, error) { return -1, nil }

func TestReadFromNegativeReader(t *testing.T) {
	t.Parallel()

	var buf Buffer
	defer func() {
		switch err := recover().(type) {
		case nil:
			t.Fatal("buffer.Buffer.ReadFrom didn't panic")
		case error:
			// this is the error string of errNegativeRead
			wantError := "buffer.Buffer: reader returned negative count from Read"
			if wantError != err.Error() {
				t.Fatalf("recovered panic: got %v, want %s", err, wantError)
			}
		default:
			t.Fatalf("unexpected panic value: %#v", err)
		}
	}()
	_, _ = buf.ReadFrom(negativeReader{})
}

func TestWriteTo(t *testing.T) {
	t.Parallel()

	var buf Buffer
	for i := 3; i < 30; i += 3 {
		s := fillBytes(t, "TestWriteTo (1)", &buf, "", 5, testBytes[:len(testBytes)/i])
		var b Buffer
		_, _ = buf.WriteTo(&b)
		empty(t, "TestWriteTo (2)", &b, s, make([]byte, len(testBytes)))
	}
}

func TestRuneIO(t *testing.T) {
	t.Parallel()

	const NRune = 1000
	// Built a test slice while we write the data
	b := make([]byte, NRune*utf8.UTFMax)
	var buf Buffer
	n := 0
	for r := rune(0); r < NRune; r++ {
		size := utf8.EncodeRune(b[n:], r)
		nbytes, err := buf.WriteRune(r)
		if err != nil || size != nbytes {
			t.Fatalf("WriteRune(%U) expected %d,nil, got %d,%v", r, size, nbytes, err)
		}

		n += size
	}
	b = b[:n]

	// Check the resulting bytes
	if !bytes.Equal(b, buf.Bytes()) {
		t.Fatalf("incorrect result from WriteRune: %q not %q", buf.Bytes(), b)
	}

	p := make([]byte, utf8.UTFMax)
	// Read it back with ReadRune
	for r := rune(0); r < NRune; r++ {
		size := utf8.EncodeRune(p, r)
		nr, nbytes, err := buf.ReadRune()
		if err != nil || r != nr || size != nbytes {
			t.Fatalf("ReadRune(%U) got %U,%d not %U,%d (err=%v)", r, nr, nbytes, r, size, err)
		}
	}

	// Check that UnreadRune works
	buf.Reset()

	// check at EOF
	if err := buf.UnreadRune(); err == nil {
		t.Fatal("UnreadRune at EOF: got no error")
	}
	if _, _, err := buf.ReadRune(); err == nil {
		t.Fatal("ReadRune at EOF: got no error")
	}
	if err := buf.UnreadRune(); err == nil {
		t.Fatal("UnreadRune after ReadRune at EOF: got no error")
	}

	// check not at EOF
	_, _ = buf.Write(b)
	for r := rune(0); r < NRune; r++ {
		r1, size, _ := buf.ReadRune()
		if err := buf.UnreadRune(); err != nil {
			t.Fatalf("UnreadRune(%U) got error %v", r, err)
		}

		r2, nbytes, err := buf.ReadRune()
		if err != nil || r1 != r2 || r != r1 || size != nbytes {
			t.Fatalf("ReadRune(%U) after UnreadRune got %U,%d not %U,%d (err=%v)", r, r2, nbytes, r, size, err)
		}
	}
}

func TestWriteInvalidRune(t *testing.T) {
	t.Parallel()

	// Invalid runes, including negative ones, should be written as
	// utf8.RuneError.
	for _, r := range []rune{-1, utf8.MaxRune + 1} {
		var buf Buffer
		_, _ = buf.WriteRune(r)
		check(t, fmt.Sprintf("TestWriteInvalidRune (%d)", r), &buf, "\uFFFD")
	}
}

func TestBufferWrite2(t *testing.T) {
	tests := []struct {
		name string
		fn   func(buf *Buffer) (int, error)
		n    int
		want string
	}{
		{
			"WriteBool",
			func(buf *Buffer) (int, error) { return buf.WriteBool(true) },
			4,
			"true",
		},
		{
			"WriteIntBase10",
			func(buf *Buffer) (int, error) { return buf.WriteInt(-42, 10) },
			3,
			"-42",
		},
		{
			"WriteIntBase16",
			func(buf *Buffer) (int, error) { return buf.WriteInt(-42, 16) },
			3,
			"-2a",
		},
		{
			"WriteUintBase10",
			func(buf *Buffer) (int, error) { return buf.WriteUint(42, 10) },
			2,
			"42",
		},
		{
			"WriteUintBase16",
			func(buf *Buffer) (int, error) { return buf.WriteUint(42, 16) },
			2,
			"2a",
		},
		{
			"WriteFloat32",
			func(buf *Buffer) (int, error) { return buf.WriteFloat(3.1415926535, 'E', -1, 32) },
			len("3.1415927E+00"),
			"3.1415927E+00",
		},
		{
			"WriteFloat64",
			func(buf *Buffer) (int, error) { return buf.WriteFloat(3.1415926535, 'E', -1, 64) },
			len("3.1415926535E+00"),
			"3.1415926535E+00",
		},
		{
			"WriteQuote",
			func(buf *Buffer) (int, error) { return buf.WriteQuote(`"Fran & Freddie's Diner"`) },
			len(strconv.Quote(`"Fran & Freddie's Diner"`)),
			strconv.Quote(`"Fran & Freddie's Diner"`),
		},
		{
			"WriteQuoteToASCII",
			func(buf *Buffer) (int, error) { return buf.WriteQuoteToASCII(`"Fran & Freddie's Diner"`) },
			len(strconv.QuoteToASCII(`"Fran & Freddie's Diner"`)),
			strconv.QuoteToASCII(`"Fran & Freddie's Diner"`),
		},
		{
			"WriteQuoteRune",
			func(buf *Buffer) (int, error) { return buf.WriteQuoteRune('☺') },
			len(strconv.QuoteRune('☺')),
			strconv.QuoteRune('☺'),
		},
		{
			"WriteQuoteRuneToASCII",
			func(buf *Buffer) (int, error) { return buf.WriteQuoteRuneToASCII('☺') },
			len(strconv.QuoteRuneToASCII('☺')),
			strconv.QuoteRuneToASCII('☺'),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf Buffer
			n, err := tc.fn(&buf)
			if err != nil {
				t.Fatalf("first call: got %v", err)
			}
			if tc.n != n {
				t.Errorf("first call: got n=%d; want %d", n, tc.n)
			}
			check(t, tc.name, &buf, tc.want)

			n, err = tc.fn(&buf)
			if err != nil {
				t.Fatalf("second call: got %v", err)
			}
			if tc.n != n {
				t.Errorf("second call: got n=%d; want %d", n, tc.n)
			}
			check(t, tc.name, &buf, tc.want+tc.want)
		})
	}
}

func TestNext(t *testing.T) {
	t.Parallel()

	b := []byte{0, 1, 2, 3, 4}
	tmp := make([]byte, 5)
	for i := 0; i <= 5; i++ {
		for j := i; j <= 5; j++ {
			for k := 0; k <= 6; k++ {
				// 0 <= i <= j <= 5; 0 <= k <= 6
				// Check that if we start with a buffer
				// of length j at offset i and ask for
				// Next(k), we get the right bytes.
				buf := NewBuffer(b[:j])
				n, _ := buf.Read(tmp[:i])
				if i != n {
					t.Fatalf("Read %d returned %d", i, n)
				}

				bb := buf.Next(k)
				want := k
				if want > j-i {
					want = j - i
				}
				if want != len(bb) {
					t.Fatalf("in %d,%d: len(Next(%d)) == %d", i, j, k, len(bb))
				}

				for l, v := range bb {
					if byte(l+i) != v {
						t.Fatalf("in %d,%d: Next(%d)[%d] = %d, want %d", i, j, k, l, v, l+i)
					}
				}
			}
		}
	}
}

var readBytesTests = []struct {
	s     string
	delim byte
	lines []string
	err   error
}{
	{"", 0, []string{""}, io.EOF},
	{"a\x00", 0, []string{"a\x00"}, nil},
	{"abbbaaaba", 'b', []string{"ab", "b", "b", "aaab"}, nil},
	{"hello\x01world", 1, []string{"hello\x01"}, nil},
	{"foo\nbar", 0, []string{"foo\nbar"}, io.EOF},
	{"alpha\nbeta\ngamma\n", '\n', []string{"alpha\n", "beta\n", "gamma\n"}, nil},
	{"alpha\nbeta\ngamma", '\n', []string{"alpha\n", "beta\n", "gamma"}, io.EOF},
}

func TestReadBytes(t *testing.T) {
	t.Parallel()

	for _, test := range readBytesTests {
		buf := NewBufferString(test.s)
		var err error

		for _, line := range test.lines {
			var bytes []byte
			bytes, err = buf.ReadBytes(test.delim)
			if line != string(bytes) {
				t.Errorf("expected %q, got %q", line, bytes)
			}
			if err != nil {
				break
			}
		}

		if test.err != err {
			t.Errorf("expected error %v, got %v", test.err, err)
		}
	}
}

func TestReadString(t *testing.T) {
	t.Parallel()

	for _, test := range readBytesTests {
		buf := NewBufferString(test.s)
		var err error

		for _, line := range test.lines {
			var s string
			s, err = buf.ReadString(test.delim)
			if line != s {
				t.Errorf("expected %q, got %q", line, s)
			}
			if err != nil {
				break
			}
		}

		if test.err != err {
			t.Errorf("expected error %v, got %v", test.err, err)
		}
	}
}

// bufferInterface is the interface Buffer implements.
type bufferInterface interface {
	Len() int
	Cap() int

	Bytes() []byte
	String() string

	Next(n int) []byte
	Read(p []byte) (n int, err error)
	ReadByte() (byte, error)
	ReadRune() (r rune, size int, err error)

	ReadFrom(r io.Reader) (n int64, err error)

	Grow(n int)
	Truncate(n int)
	Reset()

	Write(p []byte) (n int, err error)
	WriteByte(c byte) error
	WriteRune(r rune) (n int, err error)
	WriteString(s string) (n int, err error)

	WriteTo(w io.Writer) (n int64, err error)

	UnreadByte() error
	UnreadRune() error

	ReadBytes(delim byte) (line []byte, err error)
	ReadString(delim byte) (line string, err error)
}

var (
	_ bufferInterface = &bytes.Buffer{}
	_ bufferInterface = &Buffer{}
)

type bench struct {
	setup func(b *testing.B, buf bufferInterface)
	perG  func(b *testing.B, buf bufferInterface)
}

func benchBuffer(b *testing.B, bench bench) {
	for _, buf := range []bufferInterface{&bytes.Buffer{}, &Buffer{}} {
		b.Run(fmt.Sprintf("%T", buf), func(b *testing.B) {
			buf = reflect.New(reflect.TypeOf(buf).Elem()).Interface().(bufferInterface)
			if bench.setup != nil {
				bench.setup(b, buf)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				bench.perG(b, buf)
			}
		})
	}
}

func BenchmarkReadString(b *testing.B) {
	const n = 32 << 10

	data := make([]byte, n)
	data[n-1] = 'x'
	benchBuffer(b, bench{
		setup: func(b *testing.B, buf bufferInterface) {
			b.SetBytes(int64(n))
		},

		perG: func(b *testing.B, buf bufferInterface) {
			switch buf.(type) {
			case *bytes.Buffer:
				buf = bytes.NewBuffer(data)
			case *Buffer:
				buf = NewBuffer(data)
			}

			_, err := buf.ReadString('x')
			if err != nil {
				b.Fatal(err)
			}
		},
	})
}

func TestGrow(t *testing.T) {
	x := []byte{'x'}
	y := []byte{'y'}
	tmp := make([]byte, 72)
	for _, growLen := range []int{0, 100, 1000, 10000, 100000} {
		for _, startLen := range []int{0, 100, 1000, 10000, 100000} {
			xBytes := bytes.Repeat(x, startLen)

			buf := NewBuffer(xBytes)

			// If we read, this affects buf.off, which is good to test.
			readBytes, _ := buf.Read(tmp)
			yBytes := bytes.Repeat(y, growLen)
			allocs := testing.AllocsPerRun(100, func() {
				buf.Grow(growLen)
				_, _ = buf.Write(yBytes)
			})
			// Check no allocation occurs in write, as long as we're single-threaded.
			if allocs != 0 {
				t.Errorf("allocation occurred during write")
			}

			// Check that buffer has correct data.
			if !bytes.Equal(xBytes[readBytes:], buf.Bytes()[:startLen-readBytes]) {
				t.Errorf("bad initial data at %d %d", startLen, growLen)
			}
			if !bytes.Equal(yBytes, buf.Bytes()[startLen-readBytes:startLen-readBytes+growLen]) {
				t.Errorf("bad written data at %d %d", startLen, growLen)
			}
		}
	}
}

func TestGrowOverflow(t *testing.T) {
	defer func() {
		if err := recover(); ErrTooLarge != err {
			t.Errorf("after too-large Grow, recover() = %v; want %v", err, ErrTooLarge)
		}
	}()

	buf := NewBuffer(make([]byte, 1))
	const maxInt = int(^uint(0) >> 1)
	buf.Grow(maxInt)
}

func TestClip(t *testing.T) {
	t.Parallel()

	buf := NewBuffer(make([]byte, 3, 6))
	if buf.Len() != 3 {
		t.Errorf("buf.Len() == %d, want 3", buf.Len())
	}
	if buf.Cap() < 6 {
		t.Errorf("buf.Cap() == %d, want >= 6", buf.Cap())
	}

	bs := append([]byte(nil), buf.Bytes()...)
	buf.Clip()
	if !bytes.Equal(bs, buf.Bytes()) {
		t.Errorf("%v.Clip() = %v, want %[1]v", bs, buf.Bytes())
	}
	if buf.Cap() != 3 {
		t.Errorf("buf.Cap() == %d, want 3", buf.Cap())
	}
}

// Was a bug: used to give EOF reading empty slice at EOF.
func TestReadEmptyAtEOF(t *testing.T) {
	t.Parallel()

	var buf Buffer
	n, err := buf.Read(make([]byte, 0))
	if err != nil || n != 0 {
		t.Errorf("expected 0,nil, got %d,%v", n, err)
	}
}

func TestUnreadByte(t *testing.T) {
	t.Parallel()

	var buf Buffer

	// check at EOF
	if err := buf.UnreadByte(); err == nil {
		t.Fatal("UnreadByte at EOF: got no error")
	}
	if _, err := buf.ReadByte(); err == nil {
		t.Fatal("ReadByte at EOF: got no error")
	}
	if err := buf.UnreadByte(); err == nil {
		t.Fatal("UnreadByte after ReadByte at EOF: got no error")
	}

	// check not at EOF
	_, _ = buf.WriteString("abcdefghijklmnopqrstuvwxyz")

	// after unsuccessful read
	if n, err := buf.Read(nil); err != nil || n != 0 {
		t.Fatalf("Read(nil) = %d,%v; want 0,nil", n, err)
	}
	if err := buf.UnreadByte(); err == nil {
		t.Fatal("UnreadByte after Read(nil): got no error")
	}

	// after successful read
	if _, err := buf.ReadBytes('m'); err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	if err := buf.UnreadByte(); err != nil {
		t.Fatalf("UnreadByte: %v", err)
	}
	if c, err := buf.ReadByte(); err != nil || c != 'm' {
		t.Fatalf("ReadByte = %q,%v; want %q,nil", c, err, 'm')
	}
}

// Tests that we occasionally compact. Issue 5154.
func TestBufferGrowth(t *testing.T) {
	var b Buffer
	buf := make([]byte, 1024)
	_, _ = b.Write(buf[:1])

	var cap0 int
	for i := 0; i < 5<<10; i++ {
		_, _ = b.Write(buf)
		_, _ = b.Read(buf)
		if i == 0 {
			cap0 = b.Cap()
		}
	}

	cap1 := b.Cap()
	// (*Buffer).grow allows for 2x capacity slop before sliding,
	// so set our error threshold at 3x.
	if cap1 > cap0*3 {
		t.Errorf("buffer cap = %d; too big (grew from %d)", cap1, cap0)
	}
}

func TestBufferCopyPanic(t *testing.T) {
	tests := []struct {
		name      string
		fn        func()
		wantPanic bool
	}{
		{
			name: "String",
			fn: func() {
				var a Buffer
				_ = a.WriteByte('x')
				b := a
				_ = b.String() // appease vet
			},
			wantPanic: false,
		},
		{
			name: "Len",
			fn: func() {
				var a Buffer
				_ = a.WriteByte('x')
				b := a
				b.Len()
			},
			wantPanic: false,
		},
		{
			name: "Cap",
			fn: func() {
				var a Buffer
				_ = a.WriteByte('x')
				b := a
				b.Cap()
			},
			wantPanic: false,
		},
		{
			name: "Reset",
			fn: func() {
				var a Buffer
				_ = a.WriteByte('x')
				b := a
				b.Reset()
			},
			wantPanic: false,
		},
		{
			name: "Write",
			fn: func() {
				var a Buffer
				_, _ = a.Write([]byte("x"))
				b := a
				_, _ = b.Write([]byte("y"))
			},
			wantPanic: true,
		},
		{
			name: "WriteByte",
			fn: func() {
				var a Buffer
				_ = a.WriteByte('x')
				b := a
				_ = b.WriteByte('y')
			},
			wantPanic: true,
		},
		{
			name: "WriteString",
			fn: func() {
				var a Buffer
				_, _ = a.WriteString("x")
				b := a
				_, _ = b.WriteString("y")
			},
			wantPanic: true,
		},
		{
			name: "WriteRune",
			fn: func() {
				var a Buffer
				_, _ = a.WriteRune('x')
				b := a
				_, _ = b.WriteRune('y')
			},
			wantPanic: true,
		},
		{
			name: "Grow",
			fn: func() {
				var a Buffer
				a.Grow(1)
				b := a
				b.Grow(2)
			},
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		didPanic := make(chan bool, 1)
		go func() {
			defer func() { didPanic <- recover() != nil }()
			tt.fn()
		}()
		if got := <-didPanic; tt.wantPanic != got {
			t.Errorf("%s: panicked = %t; want %t", tt.name, got, tt.wantPanic)
		}
	}
}

func BenchmarkWriteByte(b *testing.B) {
	const n = 4 << 10
	benchBuffer(b, bench{
		setup: func(b *testing.B, buf bufferInterface) {
			b.SetBytes(n)
			buf.Grow(n)
		},

		perG: func(b *testing.B, buf bufferInterface) {
			buf.Reset()
			for i := 0; i < n; i++ {
				_ = buf.WriteByte('x')
			}
		},
	})
}

func BenchmarkWriteRune(b *testing.B) {
	const n = 4 << 10
	const r = '☺'
	benchBuffer(b, bench{
		setup: func(b *testing.B, buf bufferInterface) {
			b.SetBytes(int64(n * utf8.RuneLen(r)))
			buf.Grow(n * utf8.UTFMax)
		},

		perG: func(b *testing.B, buf bufferInterface) {
			buf.Reset()
			for j := 0; j < n; j++ {
				_, _ = buf.WriteRune(r)
			}
		},
	})
}

// From Issue 5154.
func BenchmarkBufferNotEmptyWriteRead(b *testing.B) {
	bs := make([]byte, 1024)
	benchBuffer(b, bench{
		perG: func(b *testing.B, buf bufferInterface) {
			buf = reflect.New(reflect.TypeOf(buf).Elem()).Interface().(bufferInterface)
			_, _ = buf.Write(bs[:1])
			for i := 0; i < 5<<10; i++ {
				_, _ = buf.Write(bs)
				_, _ = buf.Read(bs)
			}
		},
	})
}

// Check that we don't compact too often. From Issue 5154.
func BenchmarkBufferFullSmallReads(b *testing.B) {
	bs := make([]byte, 1024)
	benchBuffer(b, bench{
		perG: func(b *testing.B, buf bufferInterface) {
			buf = reflect.New(reflect.TypeOf(buf).Elem()).Interface().(bufferInterface)
			_, _ = buf.Write(bs)
			for buf.Len()+20 < buf.Cap() {
				_, _ = buf.Write(bs[:10])
			}
			for i := 0; i < 5<<10; i++ {
				_, _ = buf.Read(bs[:1])
				_, _ = buf.Write(bs[:1])
			}
		},
	})
}

func BenchmarkBufferWriteBlock(b *testing.B) {
	block := make([]byte, 1024)
	for _, n := range []int{1 << 12, 1 << 16, 1 << 20} {
		b.Run(fmt.Sprintf("N%d", n), func(b *testing.B) {
			benchBuffer(b, bench{
				setup: func(b *testing.B, buf bufferInterface) {
					b.ReportAllocs()
				},

				perG: func(b *testing.B, buf bufferInterface) {
					buf = reflect.New(reflect.TypeOf(buf).Elem()).Interface().(bufferInterface)
					for buf.Len() < n {
						_, _ = buf.Write(block)
					}
				},
			})
		})
	}
}
