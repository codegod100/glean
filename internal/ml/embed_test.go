package ml

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func float32ToBytes(vals []float32) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, vals)
	return buf.Bytes()
}

func TestBytesToFloat32s(t *testing.T) {
	want := []float32{1.0, 2.5, -3.0}
	got := BytesToFloat32s(float32ToBytes(want), len(want))
	if len(got) != len(want) {
		t.Fatalf("len got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestBytesToFloat32s_WrongSize(t *testing.T) {
	if got := BytesToFloat32s([]byte{0, 1, 2}, 4); got != nil {
		t.Fatalf("expected nil for mismatched size, got %v", got)
	}
}

func TestAvgEmbeddings(t *testing.T) {
	dim := 3
	a := []float32{2, 4, 6}
	b := []float32{4, 8, 12}
	// Average of {2,4,6} and {4,8,12} is {3,6,9}.
	out, err := AvgEmbeddings([][]byte{float32ToBytes(a), float32ToBytes(b)}, dim)
	if err != nil {
		t.Fatalf("AvgEmbeddings: %v", err)
	}
	got := BytesToFloat32s(out, dim)
	want := []float32{3, 6, 9}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestAvgEmbeddings_SkipsInvalid(t *testing.T) {
	dim := 2
	valid := []float32{4, 8}
	// nil blob and a wrong-size blob must be skipped; only the valid one counts -> its own values.
	out, err := AvgEmbeddings([][]byte{nil, {0, 1}, float32ToBytes(valid)}, dim)
	if err != nil {
		t.Fatalf("AvgEmbeddings: %v", err)
	}
	got := BytesToFloat32s(out, dim)
	for i, v := range valid {
		if got[i] != v {
			t.Fatalf("idx %d: got %v, want %v", i, got[i], v)
		}
	}
}

func TestAvgEmbeddings_AllInvalid(t *testing.T) {
	out, err := AvgEmbeddings([][]byte{nil, {0, 1}}, 2)
	if err != nil {
		t.Fatalf("AvgEmbeddings: %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil when no valid inputs, got %v", out)
	}
}
