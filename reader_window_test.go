package bitstream

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func windowTestSource() *sizedAtSource {
	data := make([]byte, 3*readerWindowSize+17)
	for i := range data {
		data[i] = byte(i*37 + i/readerWindowSize)
	}
	return &sizedAtSource{data: data, size: int64(len(data))}
}

func TestReaderWindowSequential(t *testing.T) {
	source := windowTestSource()
	reader := NewReader(source)
	for i := 0; i < len(source.data); {
		if i%7 == 0 && i+4 <= len(source.data) {
			value, err := reader.ReadUint32()
			if err != nil || value != BigEndian.Uint32(source.data[i:i+4]) {
				t.Fatalf("scalar at %d = (%x, %v)", i, value, err)
			}
			i += 4
		} else {
			value, err := reader.ReadByte()
			if err != nil || value != source.data[i] {
				t.Fatalf("byte at %d = (%x, %v)", i, value, err)
			}
			i++
		}
	}
	if source.readAtCalls != 4 {
		t.Fatalf("ReadAt calls = %d, want 4", source.readAtCalls)
	}
}

func TestReaderWindowForkAndPeek(t *testing.T) {
	source := windowTestSource()
	reader := NewReader(source)
	for range 3 {
		if _, err := reader.PeekBits(13); err != nil {
			t.Fatal(err)
		}
	}
	if source.readAtCalls != 1 || reader.BitPosition() != 0 {
		t.Fatalf("peeks: calls=%d position=%d", source.readAtCalls, reader.BitPosition())
	}
	fork, err := reader.Fork()
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.SkipBytes(readerWindowSize); err != nil {
		t.Fatal(err)
	}
	if value, err := reader.ReadByte(); err != nil || value != source.data[readerWindowSize] {
		t.Fatalf("parent refill = (%x, %v)", value, err)
	}
	if value, err := fork.ReadUint32(); err != nil || value != BigEndian.Uint32(source.data[:4]) {
		t.Fatalf("fork after parent refill = (%x, %v)", value, err)
	}
	if source.readAtCalls != 2 {
		t.Fatalf("ReadAt calls = %d, want 2", source.readAtCalls)
	}
	// Cross a window during a peek, then verify the parent cursor and data.
	if err := reader.SkipBits(uint64(readerWindowSize-2)*8 + 5); err != nil {
		t.Fatal(err)
	}
	position := reader.BitPosition()
	for range 2 {
		value, err := reader.PeekBits(32)
		if err != nil || value != referenceReadBits(source.data, position, 32, MSBFirst) {
			t.Fatalf("boundary peek = (%x, %v)", value, err)
		}
	}
	if reader.BitPosition() != position {
		t.Fatal("peek changed cursor")
	}
	value, err := reader.ReadBits(32)
	if err != nil || value != referenceReadBits(source.data, position, 32, MSBFirst) {
		t.Fatalf("read after boundary peek = (%x, %v)", value, err)
	}
}

func TestReaderWindowChildLimit(t *testing.T) {
	source := windowTestSource()
	reader := NewReader(source)
	if _, err := reader.PeekBits(1); err != nil {
		t.Fatal(err)
	}
	child, err := reader.ForkAndSkip(11)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReadBits(12); !errors.Is(err, io.ErrUnexpectedEOF) || child.BitPosition() != 11 {
		t.Fatalf("child overread: position=%d err=%v", child.BitPosition(), err)
	}
	if source.readAtCalls != 1 {
		t.Fatal("child did not use shared window")
	}
}

func TestReaderWindowLargeReadBypass(t *testing.T) {
	source := windowTestSource()
	reader := NewReader(source)
	data := make([]byte, 2*readerWindowSize)
	if n, err := reader.Read(data); n != len(data) || err != nil || !bytes.Equal(data, source.data[:len(data)]) {
		t.Fatalf("large read = (%d, %v)", n, err)
	}
	if reader.window != nil || source.readAtCalls != 1 {
		t.Fatal("large read allocated or filled a window")
	}
}

func TestReaderWindowConcurrentForks(t *testing.T) {
	data := windowTestSource().data
	reader := NewReader(io.NewSectionReader(bytes.NewReader(data), 0, int64(len(data))))
	if _, err := reader.PeekBits(1); err != nil {
		t.Fatal(err)
	}
	fork, err := reader.Fork()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	for _, cursor := range []*Reader{reader, fork} {
		go func() {
			for _, want := range data {
				value, err := cursor.ReadByte()
				if err != nil {
					done <- err
					return
				}
				if value != want {
					done <- errors.New("fork data changed during refill")
					return
				}
			}
			done <- nil
		}()
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

type partialWindowSource struct {
	*sizedAtSource
	err error
}

func (source *partialWindowSource) ReadAt(data []byte, offset int64) (int, error) {
	if offset >= 3 {
		return 0, source.err
	}
	n := copy(data, source.data[offset:3])
	return n, source.err
}

func TestReaderWindowPartialError(t *testing.T) {
	failure := errors.New("read failure")
	for _, readErr := range []error{failure, io.EOF, nil} {
		source := &partialWindowSource{sizedAtSource: windowTestSource(), err: readErr}
		reader := NewReader(source)
		if _, err := reader.ReadByte(); err != nil {
			t.Fatalf("read-ahead error surfaced before valid bytes: %v", err)
		}
		data := make([]byte, 4)
		n, err := reader.Read(data)
		wantErr := readErr
		if wantErr == nil {
			wantErr = io.ErrUnexpectedEOF
		}
		if n != 2 || !errors.Is(err, wantErr) || !bytes.Equal(data[:n], source.data[1:3]) || reader.BitPosition() != 24 {
			t.Fatalf("partial read = (%d, %v), position=%d", n, err, reader.BitPosition())
		}
	}
}
