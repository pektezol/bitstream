package bitstream

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"testing"
)

var (
	_ io.Reader       = (*Reader)(nil)
	_ io.ByteReader   = (*Reader)(nil)
	_ io.Writer       = (*Writer)(nil)
	_ io.ByteWriter   = (*Writer)(nil)
	_ io.StringWriter = (*Writer)(nil)
	_ io.Closer       = (*Writer)(nil)
)

type bitField struct {
	value uint64
	count uint8
}

func TestBitFieldsUseConfiguredBitOrder(t *testing.T) {
	tests := []struct {
		name   string
		order  BitOrder
		fields []bitField
		wire   []byte
	}{
		{
			name:  "MSB first",
			order: MSBFirst,
			fields: []bitField{
				{value: 0x8, count: 4},
				{value: 0x7, count: 3},
				{value: 0x5, count: 3},
				{value: 0x15, count: 6},
			},
			wire: []byte{0x8f, 0x55},
		},
		{
			name:  "LSB first",
			order: LSBFirst,
			fields: []bitField{
				{value: 0x5, count: 3},
				{value: 0x3, count: 2},
				{value: 0x12, count: 5},
			},
			wire: []byte{0x5d, 0x02},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			writer := NewWriter(&output, WithBitOrder(test.order))
			for _, field := range test.fields {
				if err := writer.WriteBits(field.value, field.count); err != nil {
					t.Fatalf("WriteBits(%#x, %d): %v", field.value, field.count, err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if got := output.Bytes(); !bytes.Equal(got, test.wire) {
				t.Fatalf("wire = % x, want % x", got, test.wire)
			}

			reader := NewReader(bytes.NewReader(test.wire), WithBitOrder(test.order))
			for _, field := range test.fields {
				got, err := reader.ReadBits(field.count)
				if err != nil {
					t.Fatalf("ReadBits(%d): %v", field.count, err)
				}
				if got != field.value {
					t.Fatalf("ReadBits(%d) = %#x, want %#x", field.count, got, field.value)
				}
			}
		})
	}
}

func TestBitOrderAndByteOrderAreIndependent(t *testing.T) {
	tests := []struct {
		name      string
		bitOrder  BitOrder
		byteOrder ByteOrder
		wire      []byte
	}{
		{
			name:      "MSB first with little endian scalar",
			bitOrder:  MSBFirst,
			byteOrder: LittleEndian,
			wire:      []byte{0xa6, 0x82, 0x40},
		},
		{
			name:      "LSB first with big endian scalar",
			bitOrder:  LSBFirst,
			byteOrder: BigEndian,
			wire:      []byte{0x95, 0xa0, 0x01},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			writer := NewWriter(&output, WithBitOrder(test.bitOrder), WithByteOrder(test.byteOrder))
			if err := writer.WriteBits(0x5, 3); err != nil {
				t.Fatalf("WriteBits: %v", err)
			}
			if err := writer.WriteUint16(0x1234); err != nil {
				t.Fatalf("WriteUint16: %v", err)
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if got := output.Bytes(); !bytes.Equal(got, test.wire) {
				t.Fatalf("wire = % x, want % x", got, test.wire)
			}

			reader := NewReader(bytes.NewReader(test.wire), WithBitOrder(test.bitOrder), WithByteOrder(test.byteOrder))
			prefix, err := reader.ReadBits(3)
			if err != nil {
				t.Fatalf("ReadBits: %v", err)
			}
			if prefix != 0x5 {
				t.Fatalf("prefix = %#x, want %#x", prefix, uint64(0x5))
			}
			value, err := reader.ReadUint16()
			if err != nil {
				t.Fatalf("ReadUint16: %v", err)
			}
			if value != 0x1234 {
				t.Fatalf("ReadUint16 = %#x, want %#x", value, uint16(0x1234))
			}
		})
	}
}

func TestByteOrderAliases(t *testing.T) {
	if BigEndian != binary.BigEndian {
		t.Fatalf("BigEndian = %T, want binary.BigEndian", BigEndian)
	}
	if LittleEndian != binary.LittleEndian {
		t.Fatalf("LittleEndian = %T, want binary.LittleEndian", LittleEndian)
	}
}

func TestScalarByteOrderOverrides(t *testing.T) {
	if order := NewReader(bytes.NewReader(nil)).ByteOrder(); order != binary.BigEndian {
		t.Fatalf("default Reader byte order = %T, want binary.BigEndian", order)
	}
	if order := NewWriter(io.Discard).ByteOrder(); order != binary.BigEndian {
		t.Fatalf("default Writer byte order = %T, want binary.BigEndian", order)
	}

	var output bytes.Buffer
	writer := NewWriter(&output, WithByteOrder(binary.LittleEndian))
	if order := writer.ByteOrder(); order != binary.LittleEndian {
		t.Fatalf("Writer byte order = %T, want binary.LittleEndian", order)
	}
	if err := writer.WriteUint16(0x1234); err != nil {
		t.Fatalf("WriteUint16: %v", err)
	}
	if err := writer.WriteUint16WithOrder(binary.BigEndian, 0x5678); err != nil {
		t.Fatalf("WriteUint16WithOrder: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if want := []byte{0x34, 0x12, 0x56, 0x78}; !bytes.Equal(output.Bytes(), want) {
		t.Fatalf("wire = % x, want % x", output.Bytes(), want)
	}

	reader := NewReader(bytes.NewReader(output.Bytes()), WithByteOrder(binary.LittleEndian))
	if order := reader.ByteOrder(); order != binary.LittleEndian {
		t.Fatalf("Reader byte order = %T, want binary.LittleEndian", order)
	}
	if value, err := reader.ReadUint16(); err != nil || value != 0x1234 {
		t.Fatalf("ReadUint16 = (%#x, %v), want (0x1234, nil)", value, err)
	}
	if value, err := reader.ReadUint16WithOrder(binary.BigEndian); err != nil || value != 0x5678 {
		t.Fatalf("ReadUint16WithOrder = (%#x, %v), want (0x5678, nil)", value, err)
	}
}

func TestByteIOWorksAtUnalignedPositions(t *testing.T) {
	tests := []struct {
		name  string
		order BitOrder
		wire  []byte
	}{
		{name: "MSB first", order: MSBFirst, wire: []byte{0xb5, 0x79, 0xa0}},
		{name: "LSB first", order: LSBFirst, wire: []byte{0x5d, 0x6d, 0x06}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			writer := NewWriter(&output, WithBitOrder(test.order))
			if err := writer.WriteBits(0x5, 3); err != nil {
				t.Fatalf("WriteBits: %v", err)
			}
			if count, err := writer.Write([]byte{0xab, 0xcd}); err != nil || count != 2 {
				t.Fatalf("Write = (%d, %v), want (2, nil)", count, err)
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if got := output.Bytes(); !bytes.Equal(got, test.wire) {
				t.Fatalf("wire = % x, want % x", got, test.wire)
			}

			reader := NewReader(bytes.NewReader(test.wire), WithBitOrder(test.order))
			prefix, err := reader.ReadBits(3)
			if err != nil {
				t.Fatalf("ReadBits: %v", err)
			}
			if prefix != 0x5 {
				t.Fatalf("prefix = %#x, want %#x", prefix, uint64(0x5))
			}
			data := make([]byte, 2)
			if count, err := io.ReadFull(reader, data); err != nil || count != len(data) {
				t.Fatalf("io.ReadFull = (%d, %v), want (2, nil)", count, err)
			}
			if want := []byte{0xab, 0xcd}; !bytes.Equal(data, want) {
				t.Fatalf("data = % x, want % x", data, want)
			}
		})
	}
}

func TestBitsRemaining(t *testing.T) {
	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		t.Run(orderName(order), func(t *testing.T) {
			reader := NewReader(bytes.NewReader([]byte{0xb5, 0x79}), WithBitOrder(order))
			if remaining, err := reader.BitsRemaining(); err != nil || remaining != 16 {
				t.Fatalf("initial BitsRemaining = (%d, %v), want (16, nil)", remaining, err)
			}
			if got := reader.BitPosition(); got != 0 {
				t.Fatalf("BitsRemaining changed position to %d, want 0", got)
			}

			if _, err := reader.ReadBits(3); err != nil {
				t.Fatalf("ReadBits: %v", err)
			}
			if remaining, err := reader.BitsRemaining(); err != nil || remaining != 13 {
				t.Fatalf("unaligned BitsRemaining = (%d, %v), want (13, nil)", remaining, err)
			}
			if got := reader.BitPosition(); got != 3 {
				t.Fatalf("BitsRemaining changed unaligned position to %d, want 3", got)
			}

			if err := reader.SkipBits(13); err != nil {
				t.Fatalf("SkipBits: %v", err)
			}
			if remaining, err := reader.BitsRemaining(); err != nil || remaining != 0 {
				t.Fatalf("EOF BitsRemaining = (%d, %v), want (0, nil)", remaining, err)
			}
		})
	}

	streamReader := NewReader(bytes.NewBuffer([]byte{0x80}))
	if _, err := streamReader.BitsRemaining(); !errors.Is(err, ErrRemainingBitsUnavailable) {
		t.Fatalf("stream BitsRemaining error = %v, want %v", err, ErrRemainingBitsUnavailable)
	}
	if got := streamReader.BitPosition(); got != 0 {
		t.Fatalf("unavailable BitsRemaining position = %d, want 0", got)
	}
	if value, err := streamReader.ReadByte(); err != nil || value != 0x80 {
		t.Fatalf("ReadByte after unavailable BitsRemaining = (%#x, %v), want (0x80, nil)", value, err)
	}
}

func TestReaderFork(t *testing.T) {
	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		t.Run(orderName(order), func(t *testing.T) {
			reader := NewReader(
				bytes.NewReader([]byte{0x96, 0x3c, 0xa5, 0x5a}),
				WithBitOrder(order),
				WithByteOrder(LittleEndian),
			)
			if _, err := reader.ReadBits(3); err != nil {
				t.Fatalf("ReadBits prefix: %v", err)
			}

			fork, err := reader.Fork()
			if err != nil {
				t.Fatalf("Fork: %v", err)
			}
			if got := fork.BitPosition(); got != 3 {
				t.Fatalf("fork BitPosition = %d, want 3", got)
			}
			if !fork.BitOrder().valid() || fork.BitOrder() != order {
				t.Fatalf("fork BitOrder = %v, want %v", fork.BitOrder(), order)
			}
			if got := fork.ByteOrder(); got != LittleEndian {
				t.Fatalf("fork ByteOrder = %v, want LittleEndian", got)
			}

			forkValue, err := fork.ReadBits(13)
			if err != nil {
				t.Fatalf("fork ReadBits: %v", err)
			}
			readerValue, err := reader.ReadBits(13)
			if err != nil {
				t.Fatalf("reader ReadBits: %v", err)
			}
			if readerValue != forkValue {
				t.Fatalf("independent ReadBits values = (%#x, %#x), want equal", readerValue, forkValue)
			}
			if got := reader.BitPosition(); got != 16 {
				t.Fatalf("reader BitPosition = %d, want 16", got)
			}
			if got := fork.BitPosition(); got != 16 {
				t.Fatalf("fork BitPosition = %d, want 16", got)
			}

			forkByte, err := fork.ReadByte()
			if err != nil {
				t.Fatalf("fork ReadByte: %v", err)
			}
			readerByte, err := reader.ReadByte()
			if err != nil {
				t.Fatalf("reader ReadByte: %v", err)
			}
			if readerByte != 0xa5 || forkByte != readerByte {
				t.Fatalf("independent ReadByte values = (%#x, %#x), want (0xa5, 0xa5)", readerByte, forkByte)
			}

			branch, err := fork.Fork()
			if err != nil {
				t.Fatalf("fork Fork: %v", err)
			}
			branchByte, err := branch.ReadByte()
			if err != nil {
				t.Fatalf("branch ReadByte: %v", err)
			}
			forkByte, err = fork.ReadByte()
			if err != nil {
				t.Fatalf("fork second ReadByte: %v", err)
			}
			readerByte, err = reader.ReadByte()
			if err != nil {
				t.Fatalf("reader second ReadByte: %v", err)
			}
			if branchByte != 0x5a || forkByte != branchByte || readerByte != branchByte {
				t.Fatalf("branched ReadByte values = (%#x, %#x, %#x), want (0x5a, 0x5a, 0x5a)", branchByte, forkByte, readerByte)
			}
		})
	}

	reader := NewReader(bytes.NewReader([]byte{0x12, 0x34}), WithByteOrder(LittleEndian))
	fork, err := reader.Fork()
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if value, err := fork.ReadUint16(); err != nil || value != 0x3412 {
		t.Fatalf("fork ReadUint16 = (%#x, %v), want (0x3412, nil)", value, err)
	}
	if value, err := reader.ReadUint16(); err != nil || value != 0x3412 {
		t.Fatalf("reader ReadUint16 = (%#x, %v), want (0x3412, nil)", value, err)
	}
}

func TestReaderForkPreservesBitsRemaining(t *testing.T) {
	reader := NewReader(bytes.NewReader([]byte{0x96, 0x3c, 0xa5}))
	if _, err := reader.ReadBits(3); err != nil {
		t.Fatalf("ReadBits prefix: %v", err)
	}
	fork, err := reader.Fork()
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}

	for name, current := range map[string]*Reader{"reader": reader, "fork": fork} {
		if remaining, err := current.BitsRemaining(); err != nil || remaining != 21 {
			t.Fatalf("%s BitsRemaining = (%d, %v), want (21, nil)", name, remaining, err)
		}
	}

	if err := reader.SkipBytes(1); err != nil {
		t.Fatalf("reader SkipBytes: %v", err)
	}
	if remaining, err := reader.BitsRemaining(); err != nil || remaining != 13 {
		t.Fatalf("reader BitsRemaining = (%d, %v), want (13, nil)", remaining, err)
	}
	if remaining, err := fork.BitsRemaining(); err != nil || remaining != 21 {
		t.Fatalf("fork BitsRemaining = (%d, %v), want (21, nil)", remaining, err)
	}
}

func TestReaderForkAndSkip(t *testing.T) {
	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		t.Run(orderName(order), func(t *testing.T) {
			data := []byte{0x96, 0x3c, 0xa5}
			reader := NewReader(bytes.NewReader(data), WithBitOrder(order))
			if _, err := reader.ReadBits(3); err != nil {
				t.Fatalf("ReadBits prefix: %v", err)
			}

			fork, err := reader.ForkAndSkip(1)
			if err != nil {
				t.Fatalf("ForkAndSkip: %v", err)
			}
			if got := fork.BitPosition(); got != 3 {
				t.Fatalf("fork BitPosition = %d, want 3", got)
			}
			if got := reader.BitPosition(); got != 11 {
				t.Fatalf("reader BitPosition = %d, want 11", got)
			}

			wantFork := NewReader(bytes.NewReader(data), WithBitOrder(order))
			if _, err := wantFork.ReadBits(3); err != nil {
				t.Fatalf("expected fork prefix: %v", err)
			}
			wantForkValue, err := wantFork.ReadBits(8)
			if err != nil {
				t.Fatalf("expected fork ReadBits: %v", err)
			}
			forkValue, err := fork.ReadBits(8)
			if err != nil || forkValue != wantForkValue {
				t.Fatalf("fork ReadBits = (%#x, %v), want (%#x, nil)", forkValue, err, wantForkValue)
			}

			wantReader := NewReader(bytes.NewReader(data), WithBitOrder(order))
			if _, err := wantReader.ReadBits(3); err != nil {
				t.Fatalf("expected reader prefix: %v", err)
			}
			if err := wantReader.SkipBytes(1); err != nil {
				t.Fatalf("expected reader SkipBytes: %v", err)
			}
			wantReaderValue, err := wantReader.ReadBits(8)
			if err != nil {
				t.Fatalf("expected reader ReadBits: %v", err)
			}
			readerValue, err := reader.ReadBits(8)
			if err != nil || readerValue != wantReaderValue {
				t.Fatalf("reader ReadBits = (%#x, %v), want (%#x, nil)", readerValue, err, wantReaderValue)
			}
		})
	}

	t.Run("zero bytes", func(t *testing.T) {
		reader := NewReader(bytes.NewReader([]byte{0xab}))
		fork, err := reader.ForkAndSkip(0)
		if err != nil {
			t.Fatalf("ForkAndSkip: %v", err)
		}
		if got := reader.BitPosition(); got != 0 {
			t.Fatalf("reader BitPosition = %d, want 0", got)
		}
		if got := fork.BitPosition(); got != 0 {
			t.Fatalf("fork BitPosition = %d, want 0", got)
		}
		if _, err := fork.ReadByte(); !errors.Is(err, io.EOF) {
			t.Fatalf("zero-length fork ReadByte error = %v, want io.EOF", err)
		}
		if value, err := reader.ReadByte(); err != nil || value != 0xab {
			t.Fatalf("reader ReadByte = (%#x, %v), want (0xab, nil)", value, err)
		}
	})
}

func TestReaderForkAndSkipFailure(t *testing.T) {
	t.Run("partial EOF", func(t *testing.T) {
		reader := NewReader(bytes.NewReader([]byte{0xab}))
		fork, err := reader.ForkAndSkip(2)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ForkAndSkip error = %v, want io.ErrUnexpectedEOF", err)
		}
		if fork != nil {
			t.Fatalf("ForkAndSkip fork = %v, want nil", fork)
		}
		if got := reader.BitPosition(); got != 0 {
			t.Fatalf("reader BitPosition = %d, want 0", got)
		}
		if value, err := reader.ReadByte(); err != nil || value != 0xab {
			t.Fatalf("reader ReadByte = (%#x, %v), want (0xab, nil)", value, err)
		}
	})

	t.Run("byte-count overflow", func(t *testing.T) {
		reader := NewReader(bytes.NewReader([]byte{0xab}))
		fork, err := reader.ForkAndSkip(^uint64(0)/8 + 1)
		if !errors.Is(err, ErrBitCountOverflow) {
			t.Fatalf("ForkAndSkip error = %v, want %v", err, ErrBitCountOverflow)
		}
		if got := reader.BitPosition(); got != 0 {
			t.Fatalf("reader BitPosition = %d, want 0", got)
		}
		if fork != nil {
			t.Fatalf("ForkAndSkip fork = %v, want nil", fork)
		}
		if value, err := reader.ReadByte(); err != nil || value != 0xab {
			t.Fatalf("reader ReadByte = (%#x, %v), want (0xab, nil)", value, err)
		}
	})
}

func TestReaderForkUnavailableForStreams(t *testing.T) {
	reader := NewReader(bytes.NewBuffer([]byte{0x80}))
	if fork, err := reader.Fork(); fork != nil || !errors.Is(err, ErrRandomAccessUnavailable) {
		t.Fatalf("stream Fork = (%v, %v), want (nil, %v)", fork, err, ErrRandomAccessUnavailable)
	}
	if fork, err := reader.ForkAndSkip(0); fork != nil || !errors.Is(err, ErrRandomAccessUnavailable) {
		t.Fatalf("stream ForkAndSkip = (%v, %v), want (nil, %v)", fork, err, ErrRandomAccessUnavailable)
	}
}

func TestNilReaderFork(t *testing.T) {
	var reader *Reader
	if fork, err := reader.Fork(); fork != nil || !errors.Is(err, ErrNilReader) {
		t.Fatalf("nil Reader Fork = (%v, %v), want (nil, %v)", fork, err, ErrNilReader)
	}
	fork, err := reader.ForkAndSkip(0)
	if fork != nil {
		t.Fatalf("nil Reader ForkAndSkip fork = %v, want nil", fork)
	}
	if !errors.Is(err, ErrNilReader) {
		t.Fatalf("nil Reader ForkAndSkip error = %v, want %v", err, ErrNilReader)
	}
}

func TestReadBitsToSlice(t *testing.T) {
	tests := []struct {
		name   string
		fields []bitField
		bits   uint64
		want   []byte
	}{
		{
			name: "partial byte",
			fields: []bitField{
				{value: 0x2a, count: 6},
			},
			bits: 6,
			want: []byte{0x2a},
		},
		{
			name: "exact bytes",
			fields: []bitField{
				{value: 0xab, count: 8},
				{value: 0xcd, count: 8},
			},
			bits: 16,
			want: []byte{0xab, 0xcd},
		},
		{
			name: "bytes plus remainder",
			fields: []bitField{
				{value: 0xab, count: 8},
				{value: 0xcd, count: 8},
				{value: 0x15, count: 6},
			},
			bits: 22,
			want: []byte{0xab, 0xcd, 0x15},
		},
	}

	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		t.Run(orderName(order), func(t *testing.T) {
			zeroReader := NewReader(bytes.NewReader([]byte{0xab}), WithBitOrder(order))
			zero, err := zeroReader.ReadBitsToSlice(0)
			if err != nil {
				t.Fatalf("ReadBitsToSlice(0): %v", err)
			}
			if len(zero) != 0 {
				t.Fatalf("ReadBitsToSlice(0) = % x, want empty", zero)
			}
			if got := zeroReader.BitPosition(); got != 0 {
				t.Fatalf("ReadBitsToSlice(0) position = %d, want 0", got)
			}
			if value, err := zeroReader.ReadByte(); err != nil || value != 0xab {
				t.Fatalf("ReadByte after zero read = (%#x, %v), want (0xab, nil)", value, err)
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					var output bytes.Buffer
					writer := NewWriter(&output, WithBitOrder(order))
					for _, field := range test.fields {
						if err := writer.WriteBits(field.value, field.count); err != nil {
							t.Fatalf("WriteBits(%#x, %d): %v", field.value, field.count, err)
						}
					}
					if err := writer.Close(); err != nil {
						t.Fatalf("Close: %v", err)
					}

					reader := NewReader(bytes.NewReader(output.Bytes()), WithBitOrder(order))
					got, err := reader.ReadBitsToSlice(test.bits)
					if err != nil {
						t.Fatalf("ReadBitsToSlice(%d): %v", test.bits, err)
					}
					if !bytes.Equal(got, test.want) {
						t.Fatalf("ReadBitsToSlice(%d) = % x, want % x", test.bits, got, test.want)
					}
				})
			}
		})
	}
}

func TestSliceReadersAtAlignedAndUnalignedPositions(t *testing.T) {
	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		for _, unaligned := range []bool{false, true} {
			name := "aligned"
			if unaligned {
				name = "unaligned"
			}
			t.Run(orderName(order)+"/"+name, func(t *testing.T) {
				var output bytes.Buffer
				writer := NewWriter(&output, WithBitOrder(order))
				if unaligned {
					if err := writer.WriteBits(0x5, 3); err != nil {
						t.Fatalf("WriteBits prefix: %v", err)
					}
				}
				if err := writer.WriteByte(0xab); err != nil {
					t.Fatalf("WriteByte: %v", err)
				}
				if err := writer.WriteByte(0xcd); err != nil {
					t.Fatalf("WriteByte: %v", err)
				}
				if err := writer.WriteBits(0x15, 5); err != nil {
					t.Fatalf("WriteBits remainder: %v", err)
				}
				if count, err := writer.Write([]byte{0xde, 0xad}); err != nil || count != 2 {
					t.Fatalf("Write = (%d, %v), want (2, nil)", count, err)
				}
				if err := writer.Close(); err != nil {
					t.Fatalf("Close: %v", err)
				}

				reader := NewReader(bytes.NewReader(output.Bytes()), WithBitOrder(order))
				if unaligned {
					if value, err := reader.ReadBits(3); err != nil || value != 0x5 {
						t.Fatalf("ReadBits prefix = (%#x, %v), want (0x5, nil)", value, err)
					}
				}
				bits, err := reader.ReadBitsToSlice(21)
				if err != nil {
					t.Fatalf("ReadBitsToSlice: %v", err)
				}
				if want := []byte{0xab, 0xcd, 0x15}; !bytes.Equal(bits, want) {
					t.Fatalf("ReadBitsToSlice = % x, want % x", bits, want)
				}
				data, err := reader.ReadBytesToSlice(2)
				if err != nil {
					t.Fatalf("ReadBytesToSlice: %v", err)
				}
				if want := []byte{0xde, 0xad}; !bytes.Equal(data, want) {
					t.Fatalf("ReadBytesToSlice = % x, want % x", data, want)
				}
			})
		}
	}
}

func TestSliceReaderShortReadAndOverflowBehavior(t *testing.T) {
	bitReader := NewReader(bytes.NewReader([]byte{0xab}))
	if data, err := bitReader.ReadBitsToSlice(14); !bytes.Equal(data, []byte{0xab}) || !errors.Is(err, io.EOF) {
		t.Fatalf("short ReadBitsToSlice = (% x, %v), want (ab, io.EOF)", data, err)
	}
	if got := bitReader.BitPosition(); got != 8 {
		t.Fatalf("short ReadBitsToSlice position = %d, want 8", got)
	}

	partialGroupReader := NewReader(bytes.NewReader([]byte{0xff}))
	if _, err := partialGroupReader.ReadBits(4); err != nil {
		t.Fatalf("ReadBits prefix: %v", err)
	}
	if data, err := partialGroupReader.ReadBitsToSlice(6); len(data) != 0 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short final group = (% x, %v), want (empty, io.ErrUnexpectedEOF)", data, err)
	}
	if got := partialGroupReader.BitPosition(); got != 8 {
		t.Fatalf("short final group position = %d, want 8", got)
	}

	byteReader := NewReader(bytes.NewReader([]byte{0xde, 0xad}))
	if data, err := byteReader.ReadBytesToSlice(3); !bytes.Equal(data, []byte{0xde, 0xad}) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short ReadBytesToSlice = (% x, %v), want (de ad, io.ErrUnexpectedEOF)", data, err)
	}
	if got := byteReader.BitPosition(); got != 16 {
		t.Fatalf("short ReadBytesToSlice position = %d, want 16", got)
	}

	overflowReader := NewReader(bytes.NewReader([]byte{0x80}))
	if _, err := overflowReader.ReadBytesToSlice(^uint64(0)); !errors.Is(err, ErrSliceLengthOverflow) {
		t.Fatalf("overflow ReadBytesToSlice error = %v, want %v", err, ErrSliceLengthOverflow)
	}
	if got := overflowReader.BitPosition(); got != 0 {
		t.Fatalf("overflow ReadBytesToSlice position = %d, want 0", got)
	}

	maxSliceLength := uint64(^uint(0) >> 1)
	if maxSliceLength < ^uint64(0)/8 {
		bits := (maxSliceLength + 1) * 8
		if _, err := overflowReader.ReadBitsToSlice(bits); !errors.Is(err, ErrSliceLengthOverflow) {
			t.Fatalf("overflow ReadBitsToSlice error = %v, want %v", err, ErrSliceLengthOverflow)
		}
		if got := overflowReader.BitPosition(); got != 0 {
			t.Fatalf("overflow ReadBitsToSlice position = %d, want 0", got)
		}
	}
}

func TestMustSliceReaders(t *testing.T) {
	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		t.Run(orderName(order), func(t *testing.T) {
			var output bytes.Buffer
			writer := NewWriter(&output, WithBitOrder(order))
			if err := writer.WriteByte(0xab); err != nil {
				t.Fatalf("WriteByte: %v", err)
			}
			if err := writer.WriteBits(0x15, 6); err != nil {
				t.Fatalf("WriteBits: %v", err)
			}
			if _, err := writer.Write([]byte{0xde, 0xad}); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			reader := NewReader(bytes.NewReader(output.Bytes()), WithBitOrder(order))
			if got := reader.MustReadBitsToSlice(14); !bytes.Equal(got, []byte{0xab, 0x15}) {
				t.Fatalf("MustReadBitsToSlice = % x, want ab 15", got)
			}
			if got := reader.MustReadBytesToSlice(2); !bytes.Equal(got, []byte{0xde, 0xad}) {
				t.Fatalf("MustReadBytesToSlice = % x, want de ad", got)
			}
		})
	}

	assertPanicsWithError(t, io.EOF, func() {
		NewReader(bytes.NewReader(nil)).MustReadBitsToSlice(1)
	})
	assertPanicsWithError(t, io.ErrUnexpectedEOF, func() {
		NewReader(bytes.NewReader([]byte{0})).MustReadBytesToSlice(2)
	})
	assertPanicsWithError(t, ErrSliceLengthOverflow, func() {
		NewReader(bytes.NewReader(nil)).MustReadBytesToSlice(^uint64(0))
	})
}

func TestAlignmentAndPadding(t *testing.T) {
	var output bytes.Buffer
	writer := NewWriter(&output)
	if err := writer.WriteBits(0x5, 3); err != nil {
		t.Fatalf("WriteBits: %v", err)
	}
	if padding, err := writer.PadToByte(true); err != nil || padding != 5 {
		t.Fatalf("PadToByte = (%d, %v), want (5, nil)", padding, err)
	}
	if padding, err := writer.Align(); err != nil || padding != 0 {
		t.Fatalf("Align = (%d, %v), want (0, nil)", padding, err)
	}
	if err := writer.WriteBool(true); err != nil {
		t.Fatalf("WriteBool: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if want := []byte{0xbf, 0x80}; !bytes.Equal(output.Bytes(), want) {
		t.Fatalf("wire = % x, want % x", output.Bytes(), want)
	}
	if got := writer.BitPosition(); got != 16 {
		t.Fatalf("BitPosition = %d, want 16", got)
	}
	if !writer.ByteAligned() {
		t.Fatal("Writer is not byte-aligned after Close")
	}

	reader := NewReader(bytes.NewReader([]byte{0xb5, 0xaa}))
	value, err := reader.ReadBits(3)
	if err != nil {
		t.Fatalf("ReadBits: %v", err)
	}
	if value != 0x5 {
		t.Fatalf("ReadBits = %#x, want %#x", value, uint64(0x5))
	}
	if skipped := reader.Align(); skipped != 5 {
		t.Fatalf("Align = %d, want 5", skipped)
	}
	valueByte, err := reader.ReadByte()
	if err != nil {
		t.Fatalf("ReadByte: %v", err)
	}
	if valueByte != 0xaa {
		t.Fatalf("ReadByte = %#x, want %#x", valueByte, byte(0xaa))
	}
	if got := reader.BitPosition(); got != 16 {
		t.Fatalf("BitPosition = %d, want 16", got)
	}
}

func TestStringHelpers(t *testing.T) {
	const fixed = "fixed"
	nullTerminated := string([]byte{'n', 'u', 'l', 'l', 0xff})

	var output bytes.Buffer
	writer := NewWriter(&output)
	if count, err := writer.WriteString(fixed); err != nil || count != len(fixed) {
		t.Fatalf("WriteString fixed = (%d, %v), want (%d, nil)", count, err, len(fixed))
	}
	if count, err := writer.WriteString(nullTerminated); err != nil || count != len(nullTerminated) {
		t.Fatalf("WriteString null-terminated = (%d, %v), want (%d, nil)", count, err, len(nullTerminated))
	}
	if err := writer.WriteByte(0); err != nil {
		t.Fatalf("WriteByte terminator: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reader := NewReader(bytes.NewReader(output.Bytes()))
	if value, err := reader.ReadStringToLength(uint64(len(fixed))); err != nil || value != fixed {
		t.Fatalf("ReadStringToLength = (%q, %v), want (%q, nil)", value, err, fixed)
	}
	if value, err := reader.ReadStringToNull(); err != nil || value != nullTerminated {
		t.Fatalf("ReadStringToNull = (%q, %v), want (%q, nil)", value, err, nullTerminated)
	}

	emptyReader := NewReader(bytes.NewReader([]byte{0}))
	if value, err := emptyReader.ReadStringToNull(); err != nil || value != "" {
		t.Fatalf("empty ReadStringToNull = (%q, %v), want (\"\", nil)", value, err)
	}
	if got := emptyReader.BitPosition(); got != 8 {
		t.Fatalf("empty ReadStringToNull position = %d, want 8", got)
	}

	zeroLengthReader := NewReader(bytes.NewReader(nil))
	if value, err := zeroLengthReader.ReadStringToLength(0); err != nil || value != "" {
		t.Fatalf("zero-length ReadStringToLength = (%q, %v), want (\"\", nil)", value, err)
	}

	partialNullReader := NewReader(bytes.NewReader([]byte("partial")))
	if value, err := partialNullReader.ReadStringToNull(); value != "partial" || !errors.Is(err, io.EOF) {
		t.Fatalf("unterminated ReadStringToNull = (%q, %v), want (\"partial\", io.EOF)", value, err)
	}

	partialLengthReader := NewReader(bytes.NewReader([]byte("part")))
	if value, err := partialLengthReader.ReadStringToLength(5); value != "part" || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short ReadStringToLength = (%q, %v), want (\"part\", io.ErrUnexpectedEOF)", value, err)
	}

	fixedNullReader := NewReader(bytes.NewReader([]byte{'d', 'e', 'm', 'o', 0, 'x', 'y', 'z', 0xa5}))
	if value, err := fixedNullReader.ReadStringToLength(8); err != nil || value != "demo" {
		t.Fatalf("null-terminated ReadStringToLength = (%q, %v), want (\"demo\", nil)", value, err)
	}
	if value, err := fixedNullReader.ReadByte(); err != nil || value != 0xa5 {
		t.Fatalf("ReadStringToLength did not consume its fixed-width field: ReadByte = (%#x, %v), want (0xa5, nil)", value, err)
	}

	partialFixedNullReader := NewReader(bytes.NewReader([]byte{'d', 'e', 'm', 'o', 0}))
	if value, err := partialFixedNullReader.ReadStringToLength(8); value != "demo" || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short null-terminated ReadStringToLength = (%q, %v), want (\"demo\", io.ErrUnexpectedEOF)", value, err)
	}

	overflowReader := NewReader(bytes.NewReader([]byte{0x80}))
	if _, err := overflowReader.ReadStringToLength(^uint64(0)); !errors.Is(err, ErrStringLengthOverflow) {
		t.Fatalf("overflow ReadStringToLength error = %v, want %v", err, ErrStringLengthOverflow)
	}
	if got := overflowReader.BitPosition(); got != 0 {
		t.Fatalf("overflow ReadStringToLength position = %d, want 0", got)
	}
}

func TestStringHelpersAtUnalignedPositions(t *testing.T) {
	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		t.Run(orderName(order), func(t *testing.T) {
			var output bytes.Buffer
			writer := NewWriter(&output, WithBitOrder(order))
			if err := writer.WriteBits(0x5, 3); err != nil {
				t.Fatalf("WriteBits: %v", err)
			}
			if count, err := writer.WriteString("abc"); err != nil || count != 3 {
				t.Fatalf("WriteString = (%d, %v), want (3, nil)", count, err)
			}
			if err := writer.WriteByte(0); err != nil {
				t.Fatalf("WriteByte terminator: %v", err)
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			reader := NewReader(bytes.NewReader(output.Bytes()), WithBitOrder(order))
			if value, err := reader.ReadBits(3); err != nil || value != 0x5 {
				t.Fatalf("ReadBits = (%#x, %v), want (0x5, nil)", value, err)
			}
			if value, err := reader.ReadStringToLength(3); err != nil || value != "abc" {
				t.Fatalf("ReadStringToLength = (%q, %v), want (\"abc\", nil)", value, err)
			}
			if value, err := reader.ReadStringToNull(); err != nil || value != "" {
				t.Fatalf("ReadStringToNull = (%q, %v), want (\"\", nil)", value, err)
			}
		})
	}
}

func TestMustStringHelpers(t *testing.T) {
	var output bytes.Buffer
	writer := NewWriter(&output)
	if count := writer.MustWriteString("hello"); count != 5 {
		t.Fatalf("MustWriteString = %d, want 5", count)
	}
	writer.MustWriteByte(0)
	if count := writer.MustWriteString("world"); count != 5 {
		t.Fatalf("MustWriteString = %d, want 5", count)
	}
	writer.MustClose()

	reader := NewReader(bytes.NewReader(output.Bytes()))
	if value := reader.MustReadStringToNull(); value != "hello" {
		t.Fatalf("MustReadStringToNull = %q, want \"hello\"", value)
	}
	if value := reader.MustReadStringToLength(5); value != "world" {
		t.Fatalf("MustReadStringToLength = %q, want \"world\"", value)
	}

	assertPanicsWithError(t, io.EOF, func() {
		NewReader(bytes.NewReader([]byte("unterminated"))).MustReadStringToNull()
	})
	assertPanicsWithError(t, ErrStringLengthOverflow, func() {
		NewReader(bytes.NewReader(nil)).MustReadStringToLength(^uint64(0))
	})
	assertPanicsWithError(t, io.ErrShortWrite, func() {
		NewWriter(zeroWriter{}).MustWriteString("x")
	})
}

func TestScalarHelpers(t *testing.T) {
	var output bytes.Buffer
	writer := NewWriter(&output)
	if err := writer.WriteUint8(0xfe); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteInt8(-2); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteUint16(0x1234); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteInt16WithOrder(binary.LittleEndian, -12345); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteUint32WithOrder(binary.LittleEndian, 0x89abcdef); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteInt32(-1234567); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteUint64(0x0123456789abcdef); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteInt64WithOrder(binary.LittleEndian, -123456789012345); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFloat32WithOrder(binary.LittleEndian, math.Float32frombits(0x40490fdb)); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFloat64(math.Float64frombits(0x400921fb54442d18)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	if prefix := output.Bytes()[:6]; !bytes.Equal(prefix, []byte{0xfe, 0xfe, 0x12, 0x34, 0xc7, 0xcf}) {
		t.Fatalf("scalar prefix = % x, want fe fe 12 34 c7 cf", prefix)
	}

	reader := NewReader(bytes.NewReader(output.Bytes()))
	if value, err := reader.ReadUint8(); err != nil || value != 0xfe {
		t.Fatalf("ReadUint8 = (%#x, %v), want (0xfe, nil)", value, err)
	}
	if value, err := reader.ReadInt8(); err != nil || value != -2 {
		t.Fatalf("ReadInt8 = (%d, %v), want (-2, nil)", value, err)
	}
	if value, err := reader.ReadUint16(); err != nil || value != 0x1234 {
		t.Fatalf("ReadUint16 = (%#x, %v), want (0x1234, nil)", value, err)
	}
	if value, err := reader.ReadInt16WithOrder(binary.LittleEndian); err != nil || value != -12345 {
		t.Fatalf("ReadInt16 = (%d, %v), want (-12345, nil)", value, err)
	}
	if value, err := reader.ReadUint32WithOrder(binary.LittleEndian); err != nil || value != 0x89abcdef {
		t.Fatalf("ReadUint32 = (%#x, %v), want (0x89abcdef, nil)", value, err)
	}
	if value, err := reader.ReadInt32(); err != nil || value != -1234567 {
		t.Fatalf("ReadInt32 = (%d, %v), want (-1234567, nil)", value, err)
	}
	if value, err := reader.ReadUint64(); err != nil || value != 0x0123456789abcdef {
		t.Fatalf("ReadUint64 = (%#x, %v), want (0x0123456789abcdef, nil)", value, err)
	}
	if value, err := reader.ReadInt64WithOrder(binary.LittleEndian); err != nil || value != -123456789012345 {
		t.Fatalf("ReadInt64 = (%d, %v), want (-123456789012345, nil)", value, err)
	}
	if value, err := reader.ReadFloat32WithOrder(binary.LittleEndian); err != nil || math.Float32bits(value) != 0x40490fdb {
		t.Fatalf("ReadFloat32 = (%#x, %v), want bits 0x40490fdb", math.Float32bits(value), err)
	}
	if value, err := reader.ReadFloat64(); err != nil || math.Float64bits(value) != 0x400921fb54442d18 {
		t.Fatalf("ReadFloat64 = (%#x, %v), want bits 0x400921fb54442d18", math.Float64bits(value), err)
	}
}

func TestMustReadWrappers(t *testing.T) {
	var output bytes.Buffer
	writer := NewWriter(&output)
	if err := writer.WriteBool(true); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteBits(0x3, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Align(); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteByte(0x7e); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte{0xaa, 0xbb}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteUint8(0xfe); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteInt8(-2); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteUint16(0x1234); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteInt16WithOrder(binary.LittleEndian, -12345); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteUint32WithOrder(binary.LittleEndian, 0x89abcdef); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteInt32(-1234567); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteUint64(0x0123456789abcdef); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteInt64WithOrder(binary.LittleEndian, -123456789012345); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFloat32WithOrder(binary.LittleEndian, math.Float32frombits(0x40490fdb)); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFloat64(math.Float64frombits(0x400921fb54442d18)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	reader := NewReader(bytes.NewReader(output.Bytes()))
	if !reader.MustReadBool() {
		t.Fatal("MustReadBool = false, want true")
	}
	if value := reader.MustReadBits(2); value != 0x3 {
		t.Fatalf("MustReadBits = %#x, want %#x", value, uint64(0x3))
	}
	reader.Align()
	if value := reader.MustReadByte(); value != 0x7e {
		t.Fatalf("MustReadByte = %#x, want %#x", value, byte(0x7e))
	}
	data := make([]byte, 2)
	if count, err := io.ReadFull(reader, data); err != nil || count != len(data) {
		t.Fatalf("io.ReadFull = (%d, %v), want (%d, nil)", count, err, len(data))
	}
	if want := []byte{0xaa, 0xbb}; !bytes.Equal(data, want) {
		t.Fatalf("read data = % x, want % x", data, want)
	}
	if value := reader.MustReadUint8(); value != 0xfe {
		t.Fatalf("MustReadUint8 = %#x, want %#x", value, uint8(0xfe))
	}
	if value := reader.MustReadInt8(); value != -2 {
		t.Fatalf("MustReadInt8 = %d, want -2", value)
	}
	if value := reader.MustReadUint16(); value != 0x1234 {
		t.Fatalf("MustReadUint16 = %#x, want %#x", value, uint16(0x1234))
	}
	if value := reader.MustReadInt16WithOrder(binary.LittleEndian); value != -12345 {
		t.Fatalf("MustReadInt16 = %d, want -12345", value)
	}
	if value := reader.MustReadUint32WithOrder(binary.LittleEndian); value != 0x89abcdef {
		t.Fatalf("MustReadUint32 = %#x, want %#x", value, uint32(0x89abcdef))
	}
	if value := reader.MustReadInt32(); value != -1234567 {
		t.Fatalf("MustReadInt32 = %d, want -1234567", value)
	}
	if value := reader.MustReadUint64(); value != 0x0123456789abcdef {
		t.Fatalf("MustReadUint64 = %#x, want %#x", value, uint64(0x0123456789abcdef))
	}
	if value := reader.MustReadInt64WithOrder(binary.LittleEndian); value != -123456789012345 {
		t.Fatalf("MustReadInt64 = %d, want -123456789012345", value)
	}
	if value := reader.MustReadFloat32WithOrder(binary.LittleEndian); math.Float32bits(value) != 0x40490fdb {
		t.Fatalf("MustReadFloat32 = %#x, want bits 0x40490fdb", math.Float32bits(value))
	}
	if value := reader.MustReadFloat64(); math.Float64bits(value) != 0x400921fb54442d18 {
		t.Fatalf("MustReadFloat64 = %#x, want bits 0x400921fb54442d18", math.Float64bits(value))
	}

	assertPanicsWithError(t, io.EOF, func() {
		NewReader(bytes.NewReader(nil)).MustReadByte()
	})
	assertPanicsWithError(t, ErrNilByteOrder, func() {
		NewReader(bytes.NewReader([]byte{0, 0})).MustReadUint16WithOrder(nil)
	})
}

func TestMustWriteWrappers(t *testing.T) {
	var output bytes.Buffer
	writer := NewWriter(&output)
	writer.MustWriteBool(true)
	writer.MustWriteBits(0x3, 2)
	if padding := writer.MustPadToByte(false); padding != 5 {
		t.Fatalf("MustPadToByte = %d, want 5", padding)
	}
	if padding := writer.MustAlign(); padding != 0 {
		t.Fatalf("MustAlign = %d, want 0", padding)
	}
	writer.MustWriteByte(0x7e)
	if count := writer.MustWrite([]byte{0xaa, 0xbb}); count != 2 {
		t.Fatalf("MustWrite count = %d, want 2", count)
	}
	writer.MustWriteUint8(0xfe)
	writer.MustWriteInt8(-2)
	writer.MustWriteUint16(0x1234)
	writer.MustWriteInt16WithOrder(binary.LittleEndian, -12345)
	writer.MustWriteUint32WithOrder(binary.LittleEndian, 0x89abcdef)
	writer.MustWriteInt32(-1234567)
	writer.MustWriteUint64(0x0123456789abcdef)
	writer.MustWriteInt64WithOrder(binary.LittleEndian, -123456789012345)
	writer.MustWriteFloat32WithOrder(binary.LittleEndian, math.Float32frombits(0x40490fdb))
	writer.MustWriteFloat64(math.Float64frombits(0x400921fb54442d18))
	writer.MustClose()

	reader := NewReader(bytes.NewReader(output.Bytes()))
	if value, err := reader.ReadBits(3); err != nil || value != 0x7 {
		t.Fatalf("ReadBits = (%#x, %v), want (0x7, nil)", value, err)
	}
	reader.Align()
	if value, err := reader.ReadByte(); err != nil || value != 0x7e {
		t.Fatalf("ReadByte = (%#x, %v), want (0x7e, nil)", value, err)
	}
	data := make([]byte, 2)
	if count, err := io.ReadFull(reader, data); err != nil || count != len(data) {
		t.Fatalf("io.ReadFull = (%d, %v), want (2, nil)", count, err)
	}
	if want := []byte{0xaa, 0xbb}; !bytes.Equal(data, want) {
		t.Fatalf("MustWrite data = % x, want % x", data, want)
	}
	if value, err := reader.ReadUint8(); err != nil || value != 0xfe {
		t.Fatalf("ReadUint8 = (%#x, %v), want (0xfe, nil)", value, err)
	}
	if value, err := reader.ReadInt8(); err != nil || value != -2 {
		t.Fatalf("ReadInt8 = (%d, %v), want (-2, nil)", value, err)
	}
	if value, err := reader.ReadUint16(); err != nil || value != 0x1234 {
		t.Fatalf("ReadUint16 = (%#x, %v), want (0x1234, nil)", value, err)
	}
	if value, err := reader.ReadInt16WithOrder(binary.LittleEndian); err != nil || value != -12345 {
		t.Fatalf("ReadInt16 = (%d, %v), want (-12345, nil)", value, err)
	}
	if value, err := reader.ReadUint32WithOrder(binary.LittleEndian); err != nil || value != 0x89abcdef {
		t.Fatalf("ReadUint32 = (%#x, %v), want (0x89abcdef, nil)", value, err)
	}
	if value, err := reader.ReadInt32(); err != nil || value != -1234567 {
		t.Fatalf("ReadInt32 = (%d, %v), want (-1234567, nil)", value, err)
	}
	if value, err := reader.ReadUint64(); err != nil || value != 0x0123456789abcdef {
		t.Fatalf("ReadUint64 = (%#x, %v), want (0x0123456789abcdef, nil)", value, err)
	}
	if value, err := reader.ReadInt64WithOrder(binary.LittleEndian); err != nil || value != -123456789012345 {
		t.Fatalf("ReadInt64 = (%d, %v), want (-123456789012345, nil)", value, err)
	}
	if value, err := reader.ReadFloat32WithOrder(binary.LittleEndian); err != nil || math.Float32bits(value) != 0x40490fdb {
		t.Fatalf("ReadFloat32 = (%#x, %v), want bits 0x40490fdb", math.Float32bits(value), err)
	}
	if value, err := reader.ReadFloat64(); err != nil || math.Float64bits(value) != 0x400921fb54442d18 {
		t.Fatalf("ReadFloat64 = (%#x, %v), want bits 0x400921fb54442d18", math.Float64bits(value), err)
	}

	assertPanicsWithError(t, ErrValueOverflow, func() {
		NewWriter(io.Discard).MustWriteBits(0x8, 3)
	})
	assertPanicsWithError(t, ErrNilByteOrder, func() {
		NewWriter(io.Discard).MustWriteUint16WithOrder(nil, 0x1234)
	})
	assertPanicsWithError(t, io.ErrShortWrite, func() {
		NewWriter(zeroWriter{}).MustWriteByte(0xaa)
	})
}

func TestSkipBitsAtUnalignedPosition(t *testing.T) {
	reader := NewReader(bytes.NewReader([]byte{0xf0, 0x0f, 0xaa}))
	if value, err := reader.ReadBits(3); err != nil || value != 0x7 {
		t.Fatalf("ReadBits = (%#x, %v), want (0x7, nil)", value, err)
	}
	if err := reader.SkipBits(7); err != nil {
		t.Fatalf("SkipBits: %v", err)
	}
	if value, err := reader.ReadBits(6); err != nil || value != 0x0f {
		t.Fatalf("ReadBits = (%#x, %v), want (0xf, nil)", value, err)
	}
	if got := reader.BitPosition(); got != 16 {
		t.Fatalf("BitPosition = %d, want 16", got)
	}
	if !reader.ByteAligned() {
		t.Fatal("Reader is not byte-aligned")
	}
	if err := reader.SkipBytes(1); err != nil {
		t.Fatalf("SkipBytes: %v", err)
	}
	if got := reader.BitPosition(); got != 24 {
		t.Fatalf("BitPosition = %d, want 24", got)
	}
}

func TestValidationDoesNotChangeStreamState(t *testing.T) {
	var output bytes.Buffer
	writer := NewWriter(&output)
	for _, test := range []struct {
		value uint64
		count uint8
		err   error
	}{
		{value: 0x8, count: 3, err: ErrValueOverflow},
		{value: 0, count: 0, err: ErrInvalidBitCount},
		{value: 0, count: 65, err: ErrInvalidBitCount},
	} {
		if err := writer.WriteBits(test.value, test.count); !errors.Is(err, test.err) {
			t.Fatalf("WriteBits(%#x, %d) error = %v, want %v", test.value, test.count, err, test.err)
		}
	}
	if got := writer.BitPosition(); got != 0 {
		t.Fatalf("writer position = %d, want 0", got)
	}
	if err := writer.WriteUint16WithOrder(nil, 0x1234); !errors.Is(err, ErrNilByteOrder) {
		t.Fatalf("WriteUint16WithOrder(nil) error = %v, want %v", err, ErrNilByteOrder)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := output.Bytes(); len(got) != 0 {
		t.Fatalf("wire = % x, want empty", got)
	}

	reader := NewReader(bytes.NewReader([]byte{0x80}))
	for _, count := range []uint8{0, 65} {
		if _, err := reader.ReadBits(count); !errors.Is(err, ErrInvalidBitCount) {
			t.Fatalf("ReadBits(%d) error = %v, want %v", count, err, ErrInvalidBitCount)
		}
	}
	if got := reader.BitPosition(); got != 0 {
		t.Fatalf("reader position = %d, want 0", got)
	}
	if value, err := reader.ReadBits(1); err != nil || value != 1 {
		t.Fatalf("ReadBits(1) = (%#x, %v), want (1, nil)", value, err)
	}

	invalidReader := NewReader(bytes.NewReader([]byte{0xff}), WithBitOrder(BitOrder(99)))
	if _, err := invalidReader.ReadBool(); !errors.Is(err, ErrInvalidBitOrder) {
		t.Fatalf("invalid Reader error = %v, want %v", err, ErrInvalidBitOrder)
	}
	invalidWriter := NewWriter(io.Discard, WithBitOrder(BitOrder(99)))
	if err := invalidWriter.WriteBool(true); !errors.Is(err, ErrInvalidBitOrder) {
		t.Fatalf("invalid Writer error = %v, want %v", err, ErrInvalidBitOrder)
	}
	invalidByteOrderReader := NewReader(bytes.NewReader([]byte{0, 0}), WithByteOrder(nil))
	if _, err := invalidByteOrderReader.ReadUint16(); !errors.Is(err, ErrNilByteOrder) {
		t.Fatalf("invalid byte-order Reader error = %v, want %v", err, ErrNilByteOrder)
	}
	invalidByteOrderWriter := NewWriter(io.Discard, WithByteOrder(nil))
	if err := invalidByteOrderWriter.WriteUint16(0); !errors.Is(err, ErrNilByteOrder) {
		t.Fatalf("invalid byte-order Writer error = %v, want %v", err, ErrNilByteOrder)
	}
	if err := reader.SkipBytes(^uint64(0)/8 + 1); !errors.Is(err, ErrBitCountOverflow) {
		t.Fatalf("SkipBytes overflow error = %v, want %v", err, ErrBitCountOverflow)
	}
}

func TestReadBitsDistinguishesCleanAndPartialEOF(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		prefix  uint8
		count   uint8
		wantErr error
		wantPos uint64
	}{
		{
			name:    "clean EOF at the start of a field",
			count:   1,
			wantErr: io.EOF,
			wantPos: 0,
		},
		{
			name:    "aligned truncated field",
			data:    []byte{0xa5},
			count:   9,
			wantErr: io.ErrUnexpectedEOF,
			wantPos: 8,
		},
		{
			name:    "unaligned truncated field",
			data:    []byte{0xa5},
			prefix:  3,
			count:   6,
			wantErr: io.ErrUnexpectedEOF,
			wantPos: 8,
		},
		{
			name:    "clean EOF after a complete prior field",
			data:    []byte{0xa5},
			prefix:  8,
			count:   1,
			wantErr: io.EOF,
			wantPos: 8,
		},
	}

	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		for _, test := range tests {
			t.Run(orderName(order)+"/"+test.name, func(t *testing.T) {
				reader := NewReader(bytes.NewReader(test.data), WithBitOrder(order))
				if test.prefix != 0 {
					if _, err := reader.ReadBits(test.prefix); err != nil {
						t.Fatalf("ReadBits(%d) prefix: %v", test.prefix, err)
					}
				}

				value, err := reader.ReadBits(test.count)
				if value != 0 {
					t.Fatalf("ReadBits(%d) value = %#x, want 0 on error", test.count, value)
				}
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("ReadBits(%d) error = %v, want %v", test.count, err, test.wantErr)
				}
				if got := reader.BitPosition(); got != test.wantPos {
					t.Fatalf("BitPosition = %d, want %d", got, test.wantPos)
				}
			})
		}
	}
}

func TestReadBitsPreservesNonEOFErrors(t *testing.T) {
	injected := errors.New("injected read failure")
	reader := NewReader(&errorAfterReader{data: []byte{0x80}, err: injected})
	if value, err := reader.ReadBits(9); value != 0 || !errors.Is(err, injected) {
		t.Fatalf("ReadBits = (%#x, %v), want (0, injected error)", value, err)
	}
	if got := reader.BitPosition(); got != 8 {
		t.Fatalf("BitPosition = %d, want 8", got)
	}
}

func TestEOFAndWriterFailureBehavior(t *testing.T) {
	reader := NewReader(bytes.NewReader([]byte{0x80}))
	if _, err := reader.ReadBits(9); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadBits error = %v, want io.ErrUnexpectedEOF", err)
	}
	if got := reader.BitPosition(); got != 8 {
		t.Fatalf("BitPosition = %d, want 8", got)
	}

	writer := NewWriter(zeroWriter{})
	if err := writer.WriteByte(0xaa); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("WriteByte error = %v, want io.ErrShortWrite", err)
	}
	if got := writer.BitPosition(); got != 8 {
		t.Fatalf("BitPosition = %d, want 8", got)
	}
	if err := writer.WriteBool(true); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("second write error = %v, want sticky io.ErrShortWrite", err)
	}
	if err := writer.Close(); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Close error = %v, want sticky io.ErrShortWrite", err)
	}
}

func TestRandomFieldRoundTrips(t *testing.T) {
	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		t.Run(orderName(order), func(t *testing.T) {
			random := rand.New(rand.NewPCG(1234, 5678))
			fields := make([]bitField, 256)
			for index := range fields {
				count := uint8(random.IntN(64) + 1)
				value := random.Uint64()
				if count < 64 {
					value &= uint64(1)<<count - 1
				}
				fields[index] = bitField{value: value, count: count}
			}

			var output bytes.Buffer
			writer := NewWriter(&output, WithBitOrder(order))
			for _, field := range fields {
				if err := writer.WriteBits(field.value, field.count); err != nil {
					t.Fatalf("WriteBits(%#x, %d): %v", field.value, field.count, err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			reader := NewReader(bytes.NewReader(output.Bytes()), WithBitOrder(order))
			for _, field := range fields {
				value, err := reader.ReadBits(field.count)
				if err != nil {
					t.Fatalf("ReadBits(%d): %v", field.count, err)
				}
				if value != field.value {
					t.Fatalf("ReadBits(%d) = %#x, want %#x", field.count, value, field.value)
				}
			}
		})
	}
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) {
	return 0, nil
}

func orderName(order BitOrder) string {
	if order == MSBFirst {
		return "MSB first"
	}
	return "LSB first"
}

func assertPanicsWithError(t *testing.T, want error, call func()) {
	t.Helper()
	defer func() {
		value := recover()
		if value == nil {
			t.Fatalf("call did not panic; want %v", want)
		}
		err, ok := value.(error)
		if !ok || !errors.Is(err, want) {
			t.Fatalf("panic = %v, want error matching %v", value, want)
		}
	}()
	call()
}
