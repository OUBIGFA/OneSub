package jobs

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"testing"
)

func TestStderrLineLimitDoesNotDependOnWriteBoundaries(t *testing.T) {
	const limit = 65536
	text := strings.Repeat("a", limit+100) + "end"
	for _, chunk := range []int{1, 4096, len(text) + 2} {
		t.Run(strconv.Itoa(chunk), func(t *testing.T) {
			var got []string
			output := &stderrLines{line: func(line string) { got = append(got, line) }}
			data := []byte(text + "\r\nnext\n")
			for len(data) > 0 {
				n := min(chunk, len(data))
				output.Write(data[:n])
				data = data[n:]
			}
			if len(got) != 2 || got[0] != text[len(text)-limit:] || got[1] != "next" {
				t.Fatalf("line limit changed with writes: lengths=%v", lineLengths(got))
			}
		})
	}
}

func lineLengths(lines []string) []int {
	lengths := make([]int, len(lines))
	for i, line := range lines {
		lengths[i] = len(line)
	}
	return lengths
}

func TestTailBufferKeepsExactSuffix(t *testing.T) {
	var got tailBuffer
	var all []byte
	for _, size := range []int{0, 1, 4096, 32768, 100000, 5} {
		data := bytes.Repeat([]byte{byte(size % 251)}, size)
		n, err := got.Write(data)
		all = append(all, data...)
		if n != size || err != nil || !bytes.Equal(got.data, all[max(0, len(all)-32768):]) {
			t.Fatal("tail differs from output suffix", size, n, err)
		}
	}
}

func BenchmarkTailBuffer(b *testing.B) {
	data := bytes.Repeat([]byte("x"), 4096)
	var output tailBuffer
	output.Write(bytes.Repeat([]byte("x"), 32768))
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for b.Loop() {
		output.Write(data)
	}
}

func TestControlOutputRejectsOverflowWithoutTruncatingEarlierBytes(t *testing.T) {
	output := &BoundedBuffer{Limit: 8}
	if n, err := output.Write([]byte("12345678")); n != 8 || err != nil {
		t.Fatal(n, err)
	}
	if n, err := output.Write([]byte("9")); n != 0 || err == nil {
		t.Fatal("overflow accepted", n, err)
	}
	if string(output.Bytes()) != "12345678" {
		t.Fatal("protocol prefix was discarded")
	}
}

func TestControlOutputCannotBypassLimitThroughCopy(t *testing.T) {
	output := &BoundedBuffer{Limit: 8}
	// LimitedReader hides strings.Reader.WriteTo, matching the pipe path
	// where io.Copy would otherwise use a promoted Buffer.ReadFrom method.
	_, err := io.Copy(output, io.LimitReader(strings.NewReader(strings.Repeat("x", 100)), 100))
	if err == nil || output.Len() > 8 {
		t.Fatal("copy bypassed output bound", output.Len(), err)
	}
}
