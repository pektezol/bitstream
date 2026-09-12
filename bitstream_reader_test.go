package bitstream

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"strings"
	"testing"
)

func TestReaderDetectsRandomAccessSources(t *testing.T) {
	data := []byte{0x91, 0x23, 0x45, 0x67}
	section := io.NewSectionReader(bytes.NewReader(data), 1, 2)
	hiddenPosition := bytes.NewReader(data)
	if _, err := hiddenPosition.Seek(1, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	tests := []struct {
		name string
		in   io.Reader
		want []byte
	}{
		{name: "bytes reader", in: bytes.NewReader(data), want: data},
		{name: "string reader", in: strings.NewReader(string(data)), want: data},
		{name: "section reader", in: section, want: data[1:3]},
		{name: "custom size", in: &sizedAtSource{data: data, size: int64(len(data))}, want: data},
		{name: "custom size without seeker", in: &nonSeekingSizedAtSource{reader: hiddenPosition}, want: data},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := NewReader(test.in)
			if remaining, err := reader.BitsRemaining(); err != nil || remaining != uint64(len(test.want))*8 {
				t.Fatalf("BitsRemaining = (%d, %v), want (%d, nil)", remaining, err, len(test.want)*8)
			}
			fork, err := reader.Fork()
			if err != nil {
				t.Fatalf("Fork: %v", err)
			}
			got, err := fork.ReadBytesToSlice(uint64(len(test.want)))
			if err != nil || !bytes.Equal(got, test.want) {
				t.Fatalf("fork ReadBytesToSlice = (% x, %v), want (% x, nil)", got, err, test.want)
			}
		})
	}
}

func TestReaderPrefersSizeOverStat(t *testing.T) {
	statFailure := errors.New("Stat must not be called")
	source := &sizedStatSource{
		sizedAtSource: &sizedAtSource{data: []byte{0x80}, size: 1},
		statErr:       statFailure,
	}
	reader := NewReader(source)
	if _, err := reader.Fork(); err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if source.statCalls != 0 {
		t.Fatalf("Stat calls = %d, want 0", source.statCalls)
	}
}

func TestReaderDetectsRegularFilesAndPreservesOffset(t *testing.T) {
	path := t.TempDir() + "/input.bin"
	data := []byte{0xde, 0xad, 0xbe, 0xef}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()
	if _, err := file.Seek(1, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}

	reader := NewReader(file)
	if remaining, err := reader.BitsRemaining(); err != nil || remaining != 24 {
		t.Fatalf("BitsRemaining = (%d, %v), want (24, nil)", remaining, err)
	}
	if got, err := reader.ReadBytesToSlice(2); err != nil || !bytes.Equal(got, []byte{0xad, 0xbe}) {
		t.Fatalf("ReadBytesToSlice = (% x, %v), want (ad be, nil)", got, err)
	}
	if position, err := file.Seek(0, io.SeekCurrent); err != nil || position != 1 {
		t.Fatalf("file offset after Reader reads = (%d, %v), want (1, nil)", position, err)
	}
}

func TestReaderKeepsForwardOnlyWrappersForwardOnly(t *testing.T) {
	reader := NewReader(bufio.NewReader(bytes.NewReader([]byte{0x80})))
	if _, err := reader.PeekBits(0); !errors.Is(err, ErrInvalidBitCount) {
		t.Fatalf("PeekBits(0) error = %v, want %v", err, ErrInvalidBitCount)
	}
	if _, err := reader.PeekBits(1); !errors.Is(err, ErrRandomAccessUnavailable) {
		t.Fatalf("PeekBits(1) error = %v, want %v", err, ErrRandomAccessUnavailable)
	}
	if fork, err := reader.Fork(); fork != nil || !errors.Is(err, ErrRandomAccessUnavailable) {
		t.Fatalf("Fork = (%v, %v), want (nil, %v)", fork, err, ErrRandomAccessUnavailable)
	}
	if _, err := reader.BitsRemaining(); !errors.Is(err, ErrRemainingBitsUnavailable) {
		t.Fatalf("BitsRemaining error = %v, want %v", err, ErrRemainingBitsUnavailable)
	}
	if value, err := reader.ReadBits(1); err != nil || value != 1 {
		t.Fatalf("ReadBits = (%#x, %v), want (1, nil)", value, err)
	}
}

func TestRandomAccessBoundsAndNoIOSkips(t *testing.T) {
	source := &sizedAtSource{data: []byte{0x96, 0x3c, 0xa5}, size: 3}
	reader := NewReader(source)
	source.readCalls = 0
	source.readAtCalls = 0
	source.seekCalls = 0

	if err := reader.SkipBits(3); err != nil {
		t.Fatalf("SkipBits: %v", err)
	}
	fork, err := reader.Fork()
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	child, err := reader.ForkAndSkip(8)
	if err != nil {
		t.Fatalf("ForkAndSkip: %v", err)
	}
	if remaining, err := child.BitsRemaining(); err != nil || remaining != 8 {
		t.Fatalf("child BitsRemaining = (%d, %v), want (8, nil)", remaining, err)
	}
	if source.readCalls != 0 || source.readAtCalls != 0 || source.seekCalls != 0 {
		t.Fatalf("cursor operations performed I/O: Read=%d ReadAt=%d Seek=%d", source.readCalls, source.readAtCalls, source.seekCalls)
	}
	if got := reader.BitPosition(); got != 11 {
		t.Fatalf("parent BitPosition = %d, want 11", got)
	}
	if got := fork.BitPosition(); got != 3 {
		t.Fatalf("fork BitPosition = %d, want 3", got)
	}

	want := referenceReadBits(source.data, 3, 8, MSBFirst)
	if value, err := child.ReadBits(8); err != nil || value != want {
		t.Fatalf("child ReadBits = (%#x, %v), want (%#x, nil)", value, err, want)
	}
	if source.readCalls != 0 || source.readAtCalls == 0 {
		t.Fatalf("random child read used Read=%d and ReadAt=%d, want Read=0 and ReadAt>0", source.readCalls, source.readAtCalls)
	}
	if _, err := child.ReadBool(); !errors.Is(err, io.EOF) {
		t.Fatalf("child read at bound error = %v, want io.EOF", err)
	}
}

func TestForkAndSkipNestedBoundsAndAlignment(t *testing.T) {
	data := []byte{0x96, 0x3c, 0xa5, 0x5a}
	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		t.Run(orderName(order), func(t *testing.T) {
			parent := NewReader(bytes.NewReader(data), WithBitOrder(order))
			if err := parent.SkipBits(3); err != nil {
				t.Fatalf("SkipBits: %v", err)
			}
			first, err := parent.ForkAndSkip(16)
			if err != nil {
				t.Fatalf("first ForkAndSkip: %v", err)
			}
			second, err := first.ForkAndSkip(8)
			if err != nil {
				t.Fatalf("second ForkAndSkip: %v", err)
			}
			if got := second.BitPosition(); got != 3 {
				t.Fatalf("second BitPosition = %d, want 3", got)
			}
			if remaining, err := second.BitsRemaining(); err != nil || remaining != 8 {
				t.Fatalf("second BitsRemaining = (%d, %v), want (8, nil)", remaining, err)
			}
			if value, err := second.ReadBits(8); err != nil || value != referenceReadBits(data, 3, 8, order) {
				t.Fatalf("second ReadBits = (%#x, %v), want (%#x, nil)", value, err, referenceReadBits(data, 3, 8, order))
			}
			if _, err := second.ReadBool(); !errors.Is(err, io.EOF) {
				t.Fatalf("second read past bound error = %v, want io.EOF", err)
			}
			if value, err := first.ReadBits(8); err != nil || value != referenceReadBits(data, 11, 8, order) {
				t.Fatalf("first ReadBits = (%#x, %v), want (%#x, nil)", value, err, referenceReadBits(data, 11, 8, order))
			}
			if got := parent.BitPosition(); got != 19 {
				t.Fatalf("parent BitPosition = %d, want 19", got)
			}

			alignReader := NewReader(bytes.NewReader(data), WithBitOrder(order))
			if err := alignReader.SkipBits(3); err != nil {
				t.Fatalf("align SkipBits: %v", err)
			}
			limited, err := alignReader.ForkAndSkip(8)
			if err != nil {
				t.Fatalf("align ForkAndSkip: %v", err)
			}
			if _, err := limited.ReadBits(6); err != nil {
				t.Fatalf("limited ReadBits: %v", err)
			}
			if skipped := limited.Align(); skipped != 2 {
				t.Fatalf("limited Align = %d, want 2", skipped)
			}
			if got := limited.BitPosition(); got != 11 {
				t.Fatalf("limited BitPosition = %d, want 11", got)
			}
			if remaining, err := limited.BitsRemaining(); err != nil || remaining != 0 {
				t.Fatalf("limited BitsRemaining = (%d, %v), want (0, nil)", remaining, err)
			}
		})
	}
}

func TestRandomAccessSkipBoundaries(t *testing.T) {
	reader := NewReader(bytes.NewReader([]byte{0x80}))
	if err := reader.SkipBits(12); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("oversized SkipBits error = %v, want io.ErrUnexpectedEOF", err)
	}
	if got := reader.BitPosition(); got != 8 {
		t.Fatalf("BitPosition after oversized skip = %d, want 8", got)
	}
	if err := reader.SkipBits(1); !errors.Is(err, io.EOF) {
		t.Fatalf("skip at EOF error = %v, want io.EOF", err)
	}
}

func TestBoundedReadConsumesPartialLogicalByte(t *testing.T) {
	reader := NewReader(bytes.NewReader([]byte{0x96, 0x3c}))
	if err := reader.SkipBits(3); err != nil {
		t.Fatalf("SkipBits: %v", err)
	}
	child, err := reader.ForkAndSkip(8)
	if err != nil {
		t.Fatalf("ForkAndSkip: %v", err)
	}
	if _, err := child.ReadBits(5); err != nil {
		t.Fatalf("ReadBits: %v", err)
	}
	var data [1]byte
	if count, err := child.Read(data[:]); count != 0 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("bounded Read = (%d, %v), want (0, io.ErrUnexpectedEOF)", count, err)
	}
	if got := child.BitPosition(); got != 11 {
		t.Fatalf("BitPosition after partial Read = %d, want 11", got)
	}
}

func TestPeekBitsPreservesState(t *testing.T) {
	data := []byte{0x96, 0x3c, 0xa5, 0x5a, 0x78, 0x9a, 0xbc, 0xde, 0xf0}
	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		t.Run(orderName(order), func(t *testing.T) {
			for width := uint8(1); width <= 64; width++ {
				reader := NewReader(bytes.NewReader(data), WithBitOrder(order))
				if err := reader.SkipBits(5); err != nil {
					t.Fatalf("SkipBits(%d): %v", width, err)
				}
				beforePosition := reader.BitPosition()
				beforeRemaining, err := reader.BitsRemaining()
				if err != nil {
					t.Fatalf("BitsRemaining(%d): %v", width, err)
				}
				value, err := reader.PeekBits(width)
				want := referenceReadBits(data, 5, width, order)
				if err != nil || value != want {
					t.Fatalf("PeekBits(%d) = (%#x, %v), want (%#x, nil)", width, value, err, want)
				}
				if got := reader.BitPosition(); got != beforePosition {
					t.Fatalf("PeekBits(%d) position = %d, want %d", width, got, beforePosition)
				}
				if remaining, err := reader.BitsRemaining(); err != nil || remaining != beforeRemaining {
					t.Fatalf("PeekBits(%d) remaining = (%d, %v), want (%d, nil)", width, remaining, err, beforeRemaining)
				}
				if value, err := reader.ReadBits(width); err != nil || value != want {
					t.Fatalf("ReadBits(%d) after peek = (%#x, %v), want (%#x, nil)", width, value, err, want)
				}
			}
		})
	}
}

func TestPeekBitsFailureAndMustWrapperPreserveState(t *testing.T) {
	stateReader := NewReader(bytes.NewReader([]byte{0x96, 0x3c}))
	if _, err := stateReader.ReadBits(3); err != nil {
		t.Fatalf("state ReadBits: %v", err)
	}
	beforeState := readerCursorState(stateReader)
	if value, err := stateReader.PeekBits(10); err != nil || value != referenceReadBits([]byte{0x96, 0x3c}, 3, 10, MSBFirst) {
		t.Fatalf("buffered PeekBits = (%#x, %v), want (%#x, nil)", value, err, referenceReadBits([]byte{0x96, 0x3c}, 3, 10, MSBFirst))
	}
	if afterState := readerCursorState(stateReader); afterState != beforeState {
		t.Fatalf("successful PeekBits changed buffered state from %#v to %#v", beforeState, afterState)
	}
	if first, err := stateReader.PeekBits(7); err != nil {
		t.Fatalf("first repeated PeekBits: %v", err)
	} else if second, err := stateReader.PeekBits(7); err != nil || second != first {
		t.Fatalf("repeated PeekBits = (%#x, %v), want (%#x, nil)", second, err, first)
	}

	reader := NewReader(bytes.NewReader([]byte{0xa5}))
	if err := reader.SkipBits(4); err != nil {
		t.Fatalf("SkipBits: %v", err)
	}
	beforeFailure := readerCursorState(reader)
	if value, err := reader.PeekBits(5); value != 0 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("partial PeekBits = (%#x, %v), want (0, io.ErrUnexpectedEOF)", value, err)
	}
	if afterFailure := readerCursorState(reader); afterFailure != beforeFailure {
		t.Fatalf("failed PeekBits changed state from %#v to %#v", beforeFailure, afterFailure)
	}
	if got := reader.BitPosition(); got != 4 {
		t.Fatalf("partial PeekBits position = %d, want 4", got)
	}
	if value, err := reader.ReadBits(4); err != nil || value != 0x5 {
		t.Fatalf("ReadBits after partial peek = (%#x, %v), want (0x5, nil)", value, err)
	}

	sourceErr := errors.New("read-at failure")
	source := &sizedAtSource{data: []byte{0x80}, size: 1, readAtErr: sourceErr}
	errorReader := NewReader(source)
	if _, err := errorReader.PeekBits(1); !errors.Is(err, sourceErr) {
		t.Fatalf("source-error PeekBits error = %v, want %v", err, sourceErr)
	}
	if got := errorReader.BitPosition(); got != 0 {
		t.Fatalf("source-error PeekBits position = %d, want 0", got)
	}
	source.readAtErr = nil
	if value, err := errorReader.ReadBits(1); err != nil || value != 1 {
		t.Fatalf("ReadBits after failed peek = (%#x, %v), want (1, nil)", value, err)
	}
	if _, err := errorReader.PeekBits(65); !errors.Is(err, ErrInvalidBitCount) {
		t.Fatalf("PeekBits(65) error = %v, want %v", err, ErrInvalidBitCount)
	}

	assertPanicsWithError(t, ErrInvalidBitCount, func() {
		NewReader(bytes.NewReader([]byte{0})).MustPeekBits(0)
	})
	if value := NewReader(bytes.NewReader([]byte{0x80})).MustPeekBits(1); value != 1 {
		t.Fatalf("MustPeekBits = %#x, want 1", value)
	}
	assertPanicsWithError(t, ErrRandomAccessUnavailable, func() {
		NewReader(bytes.NewBuffer([]byte{0})).MustPeekBits(1)
	})
}

func TestReaderRejectsInvalidRandomAccessExtents(t *testing.T) {
	statFailure := errors.New("stat failure")
	seekFailure := errors.New("seek failure")
	tests := []struct {
		name string
		in   io.Reader
		want error
	}{
		{
			name: "negative size",
			in:   &sizedAtSource{data: []byte{0}, size: -1},
			want: ErrInvalidSize,
		},
		{
			name: "origin beyond size",
			in:   &sizedAtSource{data: []byte{0}, size: 1, position: 2},
			want: ErrInvalidSize,
		},
		{
			name: "bit count overflow",
			in:   &sizedAtSource{size: math.MaxInt64},
			want: ErrBitCountOverflow,
		},
		{
			name: "origin error",
			in:   &sizedAtSource{data: []byte{0}, size: 1, seekErr: seekFailure},
			want: seekFailure,
		},
		{
			name: "stat error",
			in:   &statErrorAtSource{err: statFailure},
			want: statFailure,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := NewReader(test.in)
			if _, err := reader.ReadBool(); !errors.Is(err, test.want) {
				t.Fatalf("ReadBool error = %v, want %v", err, test.want)
			}
			if _, err := reader.Fork(); !errors.Is(err, test.want) {
				t.Fatalf("Fork error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestRandomAccessPrematurePhysicalEOF(t *testing.T) {
	reader := NewReader(&sizedAtSource{data: []byte{0x80}, size: 2})
	if _, err := reader.ReadBits(9); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadBits error = %v, want io.ErrUnexpectedEOF", err)
	}
	if got := reader.BitPosition(); got != 8 {
		t.Fatalf("BitPosition = %d, want 8", got)
	}
}

func TestRandomAccessCapturesBoundsOnce(t *testing.T) {
	source := &sizedAtSource{data: []byte{0x80}, size: 1}
	reader := NewReader(source)
	source.data = append(source.data, 0x40)
	source.size = 2
	if remaining, err := reader.BitsRemaining(); err != nil || remaining != 8 {
		t.Fatalf("BitsRemaining after source growth = (%d, %v), want (8, nil)", remaining, err)
	}
	if _, err := reader.ReadByte(); err != nil {
		t.Fatalf("ReadByte: %v", err)
	}
	if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
		t.Fatalf("ReadByte beyond captured bound error = %v, want io.EOF", err)
	}
}

type sizedAtSource struct {
	data        []byte
	size        int64
	position    int64
	readAtErr   error
	seekErr     error
	readCalls   int
	readAtCalls int
	seekCalls   int
}

type nonSeekingSizedAtSource struct {
	reader *bytes.Reader
}

func (source *nonSeekingSizedAtSource) Read(data []byte) (int, error) {
	return source.reader.Read(data)
}

func (source *nonSeekingSizedAtSource) ReadAt(data []byte, offset int64) (int, error) {
	return source.reader.ReadAt(data, offset)
}

func (source *nonSeekingSizedAtSource) Size() int64 {
	return source.reader.Size()
}

type readerState struct {
	current  byte
	offset   uint8
	hasByte  bool
	position uint64
	limit    uint64
	single   [1]byte
}

func readerCursorState(reader *Reader) readerState {
	return readerState{
		current:  reader.current,
		offset:   reader.offset,
		hasByte:  reader.hasByte,
		position: reader.position,
		limit:    reader.limit,
		single:   reader.single,
	}
}

func (source *sizedAtSource) Read(data []byte) (int, error) {
	source.readCalls++
	if source.position >= int64(len(source.data)) {
		return 0, io.EOF
	}
	count := copy(data, source.data[source.position:])
	source.position += int64(count)
	return count, nil
}

func (source *sizedAtSource) ReadAt(data []byte, offset int64) (int, error) {
	source.readAtCalls++
	if source.readAtErr != nil {
		return 0, source.readAtErr
	}
	if offset < 0 || offset >= int64(len(source.data)) {
		return 0, io.EOF
	}
	count := copy(data, source.data[offset:])
	if count < len(data) {
		return count, io.EOF
	}
	return count, nil
}

func (source *sizedAtSource) Seek(offset int64, whence int) (int64, error) {
	source.seekCalls++
	if source.seekErr != nil {
		return 0, source.seekErr
	}
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = source.position + offset
	case io.SeekEnd:
		next = source.size + offset
	default:
		return 0, errors.New("invalid seek whence")
	}
	if next < 0 {
		return 0, errors.New("negative seek")
	}
	source.position = next
	return next, nil
}

func (source *sizedAtSource) Size() int64 {
	return source.size
}

type statErrorAtSource struct {
	err error
}

func (source *statErrorAtSource) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (source *statErrorAtSource) ReadAt([]byte, int64) (int, error) {
	return 0, io.EOF
}

func (source *statErrorAtSource) Stat() (fs.FileInfo, error) {
	return nil, source.err
}

type sizedStatSource struct {
	*sizedAtSource
	statErr   error
	statCalls int
}

func (source *sizedStatSource) Stat() (fs.FileInfo, error) {
	source.statCalls++
	return nil, source.statErr
}

func TestNewReaderFromBytes(t *testing.T) {
	data := []byte{0x00, 0x00, 0x00}
	reader := NewReaderFromBytes(data,
		WithBitOrder(LSBFirst),
		WithByteOrder(LittleEndian),
	)

	// bytes.Reader retains data, so mutations made after construction are read.
	data[0] = 0x05
	data[1] = 0x34
	data[2] = 0x12

	if remaining, err := reader.BitsRemaining(); err != nil || remaining != 24 {
		t.Fatalf("BitsRemaining = (%d, %v), want (24, nil)", remaining, err)
	}
	if !reader.CanFork() || !reader.CanPeek() {
		t.Fatalf("byte-slice reader capabilities = (CanFork=%t, CanPeek=%t), want both true", reader.CanFork(), reader.CanPeek())
	}

	fork, err := reader.Fork()
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if value, err := reader.PeekBits(3); err != nil || value != 0x5 {
		t.Fatalf("PeekBits(3) = (%#x, %v), want (0x5, nil)", value, err)
	}
	if value, err := fork.ReadBits(3); err != nil || value != 0x5 {
		t.Fatalf("fork ReadBits(3) = (%#x, %v), want (0x5, nil)", value, err)
	}
	if value, err := reader.ReadBits(3); err != nil || value != 0x5 {
		t.Fatalf("ReadBits(3) = (%#x, %v), want (0x5, nil)", value, err)
	}
	if skipped := reader.Align(); skipped != 5 {
		t.Fatalf("Align = %d, want 5", skipped)
	}
	if value, err := reader.ReadUint16(); err != nil || value != 0x1234 {
		t.Fatalf("ReadUint16 = (%#x, %v), want (0x1234, nil)", value, err)
	}
}

func TestReaderCapabilities(t *testing.T) {
	data := []byte{0x80}
	section := io.NewSectionReader(bytes.NewReader(data), 0, int64(len(data)))
	invalidOrder := BitOrder(99)

	tests := []struct {
		name   string
		reader *Reader
		want   bool
	}{
		{name: "byte slice", reader: NewReaderFromBytes(data), want: true},
		{name: "bytes reader", reader: NewReader(bytes.NewReader(data)), want: true},
		{name: "string reader", reader: NewReader(strings.NewReader(string(data))), want: true},
		{name: "section reader", reader: NewReader(section), want: true},
		{name: "stream", reader: NewReader(bytes.NewBuffer(data)), want: false},
		{name: "nil reader", reader: nil, want: false},
		{name: "nil input", reader: NewReader(nil), want: false},
		{name: "invalid bit order", reader: NewReader(bytes.NewReader(data), WithBitOrder(invalidOrder)), want: false},
		{name: "nil byte order", reader: NewReader(bytes.NewReader(data), WithByteOrder(nil)), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.reader.CanFork(); got != test.want {
				t.Fatalf("CanFork = %t, want %t", got, test.want)
			}
			if got := test.reader.CanPeek(); got != test.want {
				t.Fatalf("CanPeek = %t, want %t", got, test.want)
			}
		})
	}
}

func TestReaderCapabilityChecksDoNotChangeState(t *testing.T) {
	reader := NewReaderFromBytes([]byte{0x96, 0x3c})
	beforePosition := reader.BitPosition()
	beforeRemaining, err := reader.BitsRemaining()
	if err != nil {
		t.Fatalf("BitsRemaining: %v", err)
	}

	assertReaderCapabilities(t, reader, true)
	assertReaderCapabilities(t, reader, true)

	if position := reader.BitPosition(); position != beforePosition {
		t.Fatalf("BitPosition after capability checks = %d, want %d", position, beforePosition)
	}
	if remaining, err := reader.BitsRemaining(); err != nil || remaining != beforeRemaining {
		t.Fatalf("BitsRemaining after capability checks = (%d, %v), want (%d, nil)", remaining, err, beforeRemaining)
	}
	if value, err := reader.ReadBits(3); err != nil || value != 0x4 {
		t.Fatalf("ReadBits(3) after capability checks = (%#x, %v), want (0x4, nil)", value, err)
	}
}

func TestReaderCapabilitiesDoNotChangeWithReaderState(t *testing.T) {
	reader := NewReaderFromBytes([]byte{0x96, 0x3c})
	assertReaderCapabilities(t, reader, true)

	if _, err := reader.ReadByte(); err != nil {
		t.Fatalf("aligned ReadByte: %v", err)
	}
	assertReaderCapabilities(t, reader, true)

	if _, err := reader.ReadBits(3); err != nil {
		t.Fatalf("unaligned ReadBits: %v", err)
	}
	assertReaderCapabilities(t, reader, true)

	fork, err := reader.Fork()
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	assertReaderCapabilities(t, reader, true)
	assertReaderCapabilities(t, fork, true)

	if _, err := reader.PeekBits(1); err != nil {
		t.Fatalf("PeekBits: %v", err)
	}
	assertReaderCapabilities(t, reader, true)

	if err := reader.SkipBits(5); err != nil {
		t.Fatalf("SkipBits to EOF: %v", err)
	}
	if _, err := reader.ReadBool(); !errors.Is(err, io.EOF) {
		t.Fatalf("ReadBool at EOF error = %v, want io.EOF", err)
	}
	assertReaderCapabilities(t, reader, true)
}

func assertReaderCapabilities(t *testing.T, reader *Reader, want bool) {
	t.Helper()
	if got := reader.CanFork(); got != want {
		t.Fatalf("CanFork = %t, want %t", got, want)
	}
	if got := reader.CanPeek(); got != want {
		t.Fatalf("CanPeek = %t, want %t", got, want)
	}
}
