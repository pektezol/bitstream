package bitstream

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func FuzzReadBitsTruncation(f *testing.F) {
	f.Add([]byte{}, uint8(0), uint8(0), uint8(0))
	f.Add([]byte{0xa5}, uint8(0), uint8(0), uint8(8))
	f.Add([]byte{0xa5}, uint8(1), uint8(3), uint8(5))
	f.Add([]byte{0xa5, 0x5a}, uint8(1), uint8(7), uint8(63))

	f.Fuzz(func(t *testing.T, data []byte, orderCode, prefix, countCode uint8) {
		data = limitFuzzData(data)
		order := fuzzBitOrder(orderCode)
		prefix %= 8
		count := countCode%64 + 1
		totalBits := uint64(len(data)) * 8
		if uint64(prefix) > totalBits {
			return
		}

		reader := NewReader(bytes.NewReader(data), WithBitOrder(order))
		if prefix != 0 {
			if _, err := reader.ReadBits(prefix); err != nil {
				t.Fatalf("ReadBits(%d) prefix: %v", prefix, err)
			}
		}

		value, err := reader.ReadBits(count)
		remaining := totalBits - uint64(prefix)
		if uint64(count) <= remaining {
			if err != nil {
				t.Fatalf("ReadBits(%d): %v", count, err)
			}
			want := referenceReadBits(data, uint64(prefix), count, order)
			if value != want {
				t.Fatalf("ReadBits(%d) = %#x, want %#x", count, value, want)
			}
			if got := reader.BitPosition(); got != uint64(prefix)+uint64(count) {
				t.Fatalf("BitPosition = %d, want %d", got, uint64(prefix)+uint64(count))
			}
			return
		}

		if value != 0 {
			t.Fatalf("truncated ReadBits(%d) = %#x, want 0", count, value)
		}
		wantErr := io.ErrUnexpectedEOF
		if remaining == 0 {
			wantErr = io.EOF
		}
		if !errors.Is(err, wantErr) {
			t.Fatalf("truncated ReadBits(%d) error = %v, want %v", count, err, wantErr)
		}
		if got := reader.BitPosition(); got != totalBits {
			t.Fatalf("BitPosition = %d, want %d", got, totalBits)
		}
	})
}

func FuzzReadBitsInjectedFailure(f *testing.F) {
	f.Add([]byte{}, uint8(0), uint8(0), uint8(0))
	f.Add([]byte{0x80}, uint8(0), uint8(0), uint8(8))
	f.Add([]byte{0xa5}, uint8(1), uint8(3), uint8(5))

	f.Fuzz(func(t *testing.T, data []byte, orderCode, prefix, countCode uint8) {
		data = limitFuzzData(data)
		order := fuzzBitOrder(orderCode)
		prefix %= 8
		count := countCode%64 + 1
		totalBits := uint64(len(data)) * 8
		if uint64(prefix) > totalBits {
			return
		}

		injected := errors.New("injected read failure")
		reader := NewReader(&errorAfterReader{data: append([]byte(nil), data...), err: injected}, WithBitOrder(order))
		if prefix != 0 {
			if _, err := reader.ReadBits(prefix); err != nil {
				t.Fatalf("ReadBits(%d) prefix: %v", prefix, err)
			}
		}

		value, err := reader.ReadBits(count)
		remaining := totalBits - uint64(prefix)
		if uint64(count) <= remaining {
			if err != nil {
				t.Fatalf("ReadBits(%d): %v", count, err)
			}
			want := referenceReadBits(data, uint64(prefix), count, order)
			if value != want {
				t.Fatalf("ReadBits(%d) = %#x, want %#x", count, value, want)
			}
			if got := reader.BitPosition(); got != uint64(prefix)+uint64(count) {
				t.Fatalf("BitPosition = %d, want %d", got, uint64(prefix)+uint64(count))
			}
			return
		}

		if value != 0 {
			t.Fatalf("failed ReadBits(%d) = %#x, want 0", count, value)
		}
		if !errors.Is(err, injected) {
			t.Fatalf("failed ReadBits(%d) error = %v, want injected error", count, err)
		}
		if got := reader.BitPosition(); got != totalBits {
			t.Fatalf("BitPosition = %d, want %d", got, totalBits)
		}
	})
}

func FuzzBoundedForkAndPeek(f *testing.F) {
	f.Add([]byte{}, uint8(0), uint8(0), uint8(0), uint8(0))
	f.Add([]byte{0xa5}, uint8(0), uint8(3), uint8(8), uint8(1))
	f.Add([]byte{0xa5, 0x5a, 0x3c}, uint8(1), uint8(5), uint8(13), uint8(2))

	f.Fuzz(func(t *testing.T, data []byte, orderCode, prefixCode, countCode, childBytesCode uint8) {
		data = limitFuzzData(data)
		order := fuzzBitOrder(orderCode)
		prefix := uint64(prefixCode % 8)
		totalBits := uint64(len(data)) * 8
		if prefix > totalBits {
			return
		}

		reader := NewReader(bytes.NewReader(data), WithBitOrder(order))
		if err := reader.SkipBits(prefix); err != nil {
			t.Fatalf("SkipBits(%d): %v", prefix, err)
		}

		count := countCode%64 + 1
		beforePosition := reader.BitPosition()
		value, err := reader.PeekBits(count)
		remaining := totalBits - prefix
		if uint64(count) <= remaining {
			want := referenceReadBits(data, prefix, count, order)
			if err != nil || value != want {
				t.Fatalf("PeekBits(%d) = (%#x, %v), want (%#x, nil)", count, value, err, want)
			}
		} else {
			wantErr := io.ErrUnexpectedEOF
			if remaining == 0 {
				wantErr = io.EOF
			}
			if value != 0 || !errors.Is(err, wantErr) {
				t.Fatalf("PeekBits(%d) = (%#x, %v), want (0, %v)", count, value, err, wantErr)
			}
		}
		if got := reader.BitPosition(); got != beforePosition {
			t.Fatalf("PeekBits changed position to %d, want %d", got, beforePosition)
		}

		childBytes := uint64(childBytesCode % 10)
		child, err := reader.ForkAndSkip(childBytes)
		childBits := childBytes * 8
		if childBits > remaining {
			if child != nil || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("ForkAndSkip(%d) = (%v, %v), want (nil, io.ErrUnexpectedEOF)", childBytes, child, err)
			}
			if got := reader.BitPosition(); got != beforePosition {
				t.Fatalf("failed ForkAndSkip changed position to %d, want %d", got, beforePosition)
			}
			return
		}
		if err != nil {
			t.Fatalf("ForkAndSkip(%d): %v", childBytes, err)
		}
		if got := reader.BitPosition(); got != prefix+childBits {
			t.Fatalf("parent BitPosition = %d, want %d", got, prefix+childBits)
		}
		if childBits == 0 {
			if _, err := child.ReadBool(); !errors.Is(err, io.EOF) {
				t.Fatalf("zero-length child read error = %v, want io.EOF", err)
			}
			return
		}
		if value, err := child.ReadBits(uint8(min(childBits, 64))); err != nil || value != referenceReadBits(data, prefix, uint8(min(childBits, 64)), order) {
			t.Fatalf("child ReadBits = (%#x, %v), want (%#x, nil)", value, err, referenceReadBits(data, prefix, uint8(min(childBits, 64)), order))
		}
	})
}

func FuzzWriteBitsInjectedFailure(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint8(0), uint8(0))
	f.Add(uint8(1), uint8(3), uint8(5), uint8(0))
	f.Add(uint8(0), uint8(7), uint8(63), uint8(8))

	f.Fuzz(func(t *testing.T, orderCode, prefix, countCode, allowedWrites uint8) {
		order := fuzzBitOrder(orderCode)
		prefix %= 8
		count := countCode%64 + 1
		allowed := int(allowedWrites % 10)
		injected := errors.New("injected write failure")
		sink := &errorAfterWriter{allowed: allowed, err: injected}
		writer := NewWriter(sink, WithBitOrder(order))
		if prefix != 0 {
			if err := writer.WriteBits(0, prefix); err != nil {
				t.Fatalf("WriteBits(%d) prefix: %v", prefix, err)
			}
		}

		err := writer.WriteBits(fuzzFieldValue(count), count)
		flushes := int((uint16(prefix) + uint16(count)) / 8)
		if allowed >= flushes {
			if err != nil {
				t.Fatalf("WriteBits(%d): %v", count, err)
			}
			if got := writer.BitPosition(); got != uint64(prefix)+uint64(count) {
				t.Fatalf("BitPosition = %d, want %d", got, uint64(prefix)+uint64(count))
			}
			return
		}

		if !errors.Is(err, injected) {
			t.Fatalf("WriteBits(%d) error = %v, want injected error", count, err)
		}
		bitsBeforeFailure := uint64(8-prefix) + uint64(allowed)*8
		if got := writer.BitPosition(); got != uint64(prefix)+bitsBeforeFailure {
			t.Fatalf("BitPosition = %d, want %d", got, uint64(prefix)+bitsBeforeFailure)
		}
		if err := writer.WriteBool(true); !errors.Is(err, injected) {
			t.Fatalf("write after failure error = %v, want injected error", err)
		}
	})
}

func limitFuzzData(data []byte) []byte {
	const maxFuzzData = 64
	if len(data) > maxFuzzData {
		return data[:maxFuzzData]
	}
	return data
}

func fuzzBitOrder(code uint8) BitOrder {
	if code&1 == 0 {
		return MSBFirst
	}
	return LSBFirst
}

func fuzzFieldValue(count uint8) uint64 {
	const value = uint64(0x9e3779b97f4a7c15)
	if count == 64 {
		return value
	}
	return value & (uint64(1)<<count - 1)
}

func referenceReadBits(data []byte, start uint64, count uint8, order BitOrder) uint64 {
	var value uint64
	for index := range count {
		position := start + uint64(index)
		byteIndex := position / 8
		offset := uint8(position % 8)
		shift := offset
		if order == MSBFirst {
			shift = 7 - offset
		}
		bit := data[byteIndex]&(1<<shift) != 0
		if order == MSBFirst {
			value <<= 1
			if bit {
				value |= 1
			}
		} else if bit {
			value |= uint64(1) << index
		}
	}
	return value
}

type errorAfterReader struct {
	data []byte
	err  error
}

func (reader *errorAfterReader) Read(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if len(reader.data) == 0 {
		return 0, reader.err
	}
	data[0] = reader.data[0]
	reader.data = reader.data[1:]
	return 1, nil
}

type errorAfterWriter struct {
	allowed int
	err     error
}

func (writer *errorAfterWriter) Write(data []byte) (int, error) {
	if writer.allowed == 0 {
		return 0, writer.err
	}
	writer.allowed--
	return len(data), nil
}
