package buffer_test

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"

	"github.com/weiwenchen2022/buffer"
)

func ExampleBuffer() {
	var b buffer.Buffer // A Buffer needs no initialization.
	b.Write([]byte("Hello "))
	fmt.Fprintf(&b, "world!")
	b.WriteTo(os.Stdout)
	// Output:
	// Hello world!
}

func ExampleBuffer_reader() {
	// A Buffer can turn a string or a []byte into an io.Reader.
	buf := buffer.NewBufferString("R29waGVycyBydWxlIQ==")
	dec := base64.NewDecoder(base64.StdEncoding, buf)
	_, _ = io.Copy(os.Stdout, dec)
	// Output:
	// Gophers rule!
}

func ExampleBuffer_Bytes() {
	buf := buffer.Buffer{}
	buf.Write([]byte{'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd'})
	os.Stdout.Write(buf.Bytes())
	// Output:
	// hello world
}

func ExampleBuffer_Cap() {
	buf1 := buffer.NewBuffer(make([]byte, 10))
	buf2 := buffer.NewBuffer(make([]byte, 0, 10))
	fmt.Println(buf1.Cap())
	fmt.Println(buf2.Cap())
	// Output:
	// 10
	// 10
}

func ExampleBuffer_Grow() {
	var b buffer.Buffer
	b.Grow(64)
	b.Write([]byte("64 bytes or fewer"))
	fmt.Printf("%q", b.Bytes())
	// Output:
	// "64 bytes or fewer"
}

func ExampleBuffer_Len() {
	var b buffer.Buffer
	b.Grow(64)
	b.Write([]byte("abcde"))
	fmt.Printf("%d", b.Len())
	// Output:
	// 5
}

func ExampleBuffer_Next() {
	var b buffer.Buffer
	b.Grow(64)
	b.Write([]byte("abcde"))
	fmt.Printf("%s\n", string(b.Next(2)))
	fmt.Printf("%s\n", string(b.Next(2)))
	fmt.Printf("%s", string(b.Next(2)))
	// Output:
	// ab
	// cd
	// e
}

func ExampleBuffer_Read() {
	var b buffer.Buffer
	b.Grow(64)
	b.Write([]byte("abcde"))
	rdbuf := make([]byte, 1)
	n, err := b.Read(rdbuf)
	if err != nil {
		panic(err)
	}
	fmt.Println(n)
	fmt.Println(b.String())
	fmt.Println(string(rdbuf))
	// Output:
	// 1
	// bcde
	// a
}

func ExampleBuffer_ReadByte() {
	var b buffer.Buffer
	b.Grow(64)
	b.Write([]byte("abcde"))
	c, err := b.ReadByte()
	if err != nil {
		panic(err)
	}
	fmt.Println(c)
	fmt.Println(b.String())
	// Output:
	// 97
	// bcde
}
