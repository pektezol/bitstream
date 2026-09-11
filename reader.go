package bitstream

import (
	"bytes"
	"io"
	"math"
)

// Reader reads a sequence of bits from an io.Reader.
//
// Reader is not safe for concurrent use. It implements io.Reader and
// io.ByteReader. Its byte-oriented methods assemble the next eight stream
// bits into each returned byte, so they work at non-byte-aligned positions.
type Reader struct {
	in        io.Reader
	order     BitOrder
	byteOrder ByteOrder
	current   byte
	offset    uint8
	hasByte   bool
	position  uint64
	initErr   error
	single    [1]byte
}

// NewReader returns a Reader that reads from in. Its defaults are MSBFirst and
// BigEndian; configure them with WithBitOrder and WithByteOrder.
func NewReader(in io.Reader, options ...Option) *Reader {
	config, err := newConfig(options)
	if err == nil && in == nil {
		err = ErrNilReader
	}
	return &Reader{
		in:        in,
		order:     config.bitOrder,
		byteOrder: config.byteOrder,
		initErr:   err,
	}
}

// NewReaderFromBytes returns a Reader over data. The data is not copied.
func NewReaderFromBytes(data []byte, options ...Option) *Reader {
	return NewReader(bytes.NewReader(data), options...)
}

// ReadBool reads one bit and returns true for a set bit.
func (reader *Reader) ReadBool() (bool, error) {
	return reader.readBit()
}

// ReadBits reads count bits and returns them in the low count bits of the
// result. count must be between 1 and 64.
//
// With MSBFirst, the first stream bit becomes bit count-1 of the result. With
// LSBFirst, it becomes bit 0. If EOF occurs before any requested bit is read,
// ReadBits returns io.EOF. If EOF occurs after one or more requested bits have
// been consumed, it returns io.ErrUnexpectedEOF. Other underlying errors are
// returned unchanged. On any read error, Reader remains advanced by the bits
// already consumed and ReadBits returns zero.
func (reader *Reader) ReadBits(count uint8) (uint64, error) {
	if err := reader.readable(); err != nil {
		return 0, err
	}
	if count == 0 || count > 64 {
		return 0, ErrInvalidBitCount
	}

	var value uint64
	if reader.order == MSBFirst {
		for index := uint8(0); index < count; index++ {
			bit, err := reader.readBit()
			if err != nil {
				return 0, readBitsError(err, index)
			}
			value <<= 1
			if bit {
				value |= 1
			}
		}
		return value, nil
	}

	for index := uint8(0); index < count; index++ {
		bit, err := reader.readBit()
		if err != nil {
			return 0, readBitsError(err, index)
		}
		if bit {
			value |= uint64(1) << index
		}
	}
	return value, nil
}

func readBitsError(err error, consumed uint8) error {
	if consumed > 0 && err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

// ReadByte reads the next eight stream bits as a byte. It implements
// io.ByteReader.
func (reader *Reader) ReadByte() (byte, error) {
	value, err := reader.ReadBits(8)
	return byte(value), err
}

// Read reads up to len(data) logical bytes from the bit stream. It implements
// io.Reader. At a byte boundary it reads directly from the underlying source;
// otherwise it assembles each byte from the next eight stream bits.
func (reader *Reader) Read(data []byte) (int, error) {
	if err := reader.readable(); err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return 0, nil
	}

	if !reader.hasByte {
		count, err := reader.in.Read(data)
		if count < 0 || count > len(data) {
			return 0, errInvalidRead
		}
		reader.position += uint64(count) * 8
		return count, err
	}

	for index := range data {
		value, err := reader.ReadByte()
		if err != nil {
			return index, err
		}
		data[index] = value
	}
	return len(data), nil
}

// ReadBitsToSlice reads bits into a newly allocated packed byte slice. It
// returns ceil(bits / 8) bytes. Each complete group of eight bits is a logical
// byte; when bits is not a multiple of eight, the final byte contains the
// remaining field value in its low bits according to ReadBits semantics.
//
// If the stream ends first, it returns the completed packed bytes and the read
// error. A final partial group is omitted when it cannot be read completely.
// bits must produce a slice length that fits in a Go int.
//
// This method allocates the requested result before reading. Callers must
// validate input-derived bit counts against an application-specific allocation
// limit before calling it.
func (reader *Reader) ReadBitsToSlice(bits uint64) ([]byte, error) {
	if err := reader.readable(); err != nil {
		return nil, err
	}

	length := bits / 8
	if bits%8 != 0 {
		length++
	}
	if length > uint64(^uint(0)>>1) {
		return nil, ErrSliceLengthOverflow
	}

	data := make([]byte, int(length))
	fullBytes := int(bits / 8)
	for index := range fullBytes {
		value, err := reader.ReadByte()
		if err != nil {
			return data[:index], err
		}
		data[index] = value
	}

	if remaining := uint8(bits % 8); remaining != 0 {
		value, err := reader.ReadBits(remaining)
		if err != nil {
			return data[:fullBytes], err
		}
		data[fullBytes] = byte(value)
	}
	return data, nil
}

// ReadBytesToSlice reads count logical bytes into a newly allocated slice.
// If the stream ends first, it returns the bytes read and the read error.
// count must fit in a Go slice length.
//
// This method allocates the requested result before reading. Callers must
// validate input-derived byte counts against an application-specific allocation
// limit before calling it.
func (reader *Reader) ReadBytesToSlice(count uint64) ([]byte, error) {
	if err := reader.readable(); err != nil {
		return nil, err
	}
	if count > uint64(^uint(0)>>1) {
		return nil, ErrSliceLengthOverflow
	}

	data := make([]byte, int(count))
	read, err := io.ReadFull(reader, data)
	return data[:read], err
}

// ReadStringToNull reads logical bytes until it encounters a null byte. The
// null byte is consumed but not included in the returned string. If the
// stream ends first, it returns the partial string and the read error.
//
// This method has no maximum length. Callers reading untrusted input should
// bound the input or otherwise enforce an application-specific string limit.
func (reader *Reader) ReadStringToNull() (string, error) {
	if err := reader.readable(); err != nil {
		return "", err
	}

	var data []byte
	for {
		value, err := reader.ReadByte()
		if err != nil {
			return string(data), err
		}
		if value == 0 {
			return string(data), nil
		}
		data = append(data, value)
	}
}

// ReadStringToLength reads length logical bytes and returns the bytes through
// the first null byte as a string. It still consumes all length bytes, so a
// null-terminated string stored in a fixed-width field does not misalign the
// next read. If the stream ends first, it returns the partial string and the
// read error. length must fit in a Go slice length.
//
// This method allocates result capacity for length bytes before reading.
// Callers must validate input-derived lengths against an application-specific
// allocation limit before calling it.
func (reader *Reader) ReadStringToLength(length uint64) (string, error) {
	if err := reader.readable(); err != nil {
		return "", err
	}
	if length > uint64(^uint(0)>>1) {
		return "", ErrStringLengthOverflow
	}

	data := make([]byte, 0, int(length))
	for index := uint64(0); index < length; index++ {
		value, err := reader.ReadByte()
		if err != nil {
			if index > 0 && err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return string(data), err
		}
		if value == 0 {
			if err := reader.SkipBytes(length - index - 1); err != nil {
				if err == io.EOF {
					err = io.ErrUnexpectedEOF
				}
				return string(data), err
			}
			return string(data), nil
		}
		data = append(data, value)
	}
	return string(data), nil
}

// SkipBits consumes count bits without returning them. If an I/O error occurs,
// Reader remains advanced by every bit consumed before that error.
func (reader *Reader) SkipBits(count uint64) error {
	if err := reader.readable(); err != nil {
		return err
	}

	if reader.hasByte && count > 0 {
		remaining := uint64(8 - reader.offset)
		consumed := min(count, remaining)
		reader.offset += uint8(consumed)
		reader.position += consumed
		count -= consumed
		if reader.offset == 8 {
			reader.clearCurrentByte()
		}
	}

	bytesToSkip := count / 8
	if bytesToSkip > 0 {
		if err := reader.discardBytes(bytesToSkip); err != nil {
			return err
		}
		count -= bytesToSkip * 8
	}

	for range count {
		if _, err := reader.readBit(); err != nil {
			return err
		}
	}
	return nil
}

// SkipBytes consumes count logical bytes without returning them.
func (reader *Reader) SkipBytes(count uint64) error {
	const maxUint64 = ^uint64(0)
	if count > maxUint64/8 {
		return ErrBitCountOverflow
	}
	return reader.SkipBits(count * 8)
}

// Align discards the unread bits in the current byte and returns their count.
// If Reader is already byte-aligned, it returns zero.
func (reader *Reader) Align() uint8 {
	if reader == nil || !reader.hasByte {
		return 0
	}
	skipped := 8 - reader.offset
	reader.position += uint64(skipped)
	reader.clearCurrentByte()
	return skipped
}

// BitPosition reports the number of stream bits consumed, including bits
// discarded by Align.
func (reader *Reader) BitPosition() uint64 {
	if reader == nil {
		return 0
	}
	return reader.position
}

// BitsRemaining reports the number of unread stream bits through EOF without
// consuming input. It includes unread bits in Reader's current buffered byte.
// The underlying source must implement io.Seeker; otherwise it returns
// ErrRemainingBitsUnavailable.
func (reader *Reader) BitsRemaining() (uint64, error) {
	if err := reader.readable(); err != nil {
		return 0, err
	}
	seeker, ok := reader.in.(io.Seeker)
	if !ok {
		return 0, ErrRemainingBitsUnavailable
	}

	current, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	end, endErr := seeker.Seek(0, io.SeekEnd)
	_, restoreErr := seeker.Seek(current, io.SeekStart)
	if restoreErr != nil {
		return 0, restoreErr
	}
	if endErr != nil {
		return 0, endErr
	}
	if current < 0 || end < current {
		return 0, ErrRemainingBitsUnavailable
	}

	remainingBytes := uint64(end - current)
	if remainingBytes > ^uint64(0)/8 {
		return 0, ErrBitCountOverflow
	}
	remaining := remainingBytes * 8
	if reader.hasByte {
		remaining += uint64(8 - reader.offset)
	}
	return remaining, nil
}

// ByteAligned reports whether Reader is positioned at a byte boundary.
func (reader *Reader) ByteAligned() bool {
	return reader == nil || !reader.hasByte
}

// BitOrder reports Reader's configured bit order.
func (reader *Reader) BitOrder() BitOrder {
	if reader == nil {
		return MSBFirst
	}
	return reader.order
}

// ByteOrder reports Reader's default byte order for fixed-width scalar
// methods.
func (reader *Reader) ByteOrder() ByteOrder {
	if reader == nil {
		return BigEndian
	}
	return reader.byteOrder
}

// ReadUint8 reads one unsigned byte.
func (reader *Reader) ReadUint8() (uint8, error) {
	value, err := reader.ReadByte()
	return uint8(value), err
}

// ReadInt8 reads one signed byte.
func (reader *Reader) ReadInt8() (int8, error) {
	value, err := reader.ReadByte()
	return int8(value), err
}

// ReadUint16 reads an unsigned 16-bit scalar in Reader's default byte order.
func (reader *Reader) ReadUint16() (uint16, error) {
	if reader == nil {
		return 0, ErrNilReader
	}
	return reader.ReadUint16WithOrder(reader.byteOrder)
}

// ReadUint16WithOrder reads an unsigned 16-bit scalar using order rather than
// Reader's default byte order.
func (reader *Reader) ReadUint16WithOrder(order ByteOrder) (uint16, error) {
	if order == nil {
		return 0, ErrNilByteOrder
	}
	var data [2]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		return 0, err
	}
	return order.Uint16(data[:]), nil
}

// ReadInt16 reads a signed 16-bit scalar in Reader's default byte order.
func (reader *Reader) ReadInt16() (int16, error) {
	value, err := reader.ReadUint16()
	return int16(value), err
}

// ReadInt16WithOrder reads a signed 16-bit scalar using order rather than
// Reader's default byte order.
func (reader *Reader) ReadInt16WithOrder(order ByteOrder) (int16, error) {
	value, err := reader.ReadUint16WithOrder(order)
	return int16(value), err
}

// ReadUint32 reads an unsigned 32-bit scalar in Reader's default byte order.
func (reader *Reader) ReadUint32() (uint32, error) {
	if reader == nil {
		return 0, ErrNilReader
	}
	return reader.ReadUint32WithOrder(reader.byteOrder)
}

// ReadUint32WithOrder reads an unsigned 32-bit scalar using order rather than
// Reader's default byte order.
func (reader *Reader) ReadUint32WithOrder(order ByteOrder) (uint32, error) {
	if order == nil {
		return 0, ErrNilByteOrder
	}
	var data [4]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		return 0, err
	}
	return order.Uint32(data[:]), nil
}

// ReadInt32 reads a signed 32-bit scalar in Reader's default byte order.
func (reader *Reader) ReadInt32() (int32, error) {
	value, err := reader.ReadUint32()
	return int32(value), err
}

// ReadInt32WithOrder reads a signed 32-bit scalar using order rather than
// Reader's default byte order.
func (reader *Reader) ReadInt32WithOrder(order ByteOrder) (int32, error) {
	value, err := reader.ReadUint32WithOrder(order)
	return int32(value), err
}

// ReadUint64 reads an unsigned 64-bit scalar in Reader's default byte order.
func (reader *Reader) ReadUint64() (uint64, error) {
	if reader == nil {
		return 0, ErrNilReader
	}
	return reader.ReadUint64WithOrder(reader.byteOrder)
}

// ReadUint64WithOrder reads an unsigned 64-bit scalar using order rather than
// Reader's default byte order.
func (reader *Reader) ReadUint64WithOrder(order ByteOrder) (uint64, error) {
	if order == nil {
		return 0, ErrNilByteOrder
	}
	var data [8]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		return 0, err
	}
	return order.Uint64(data[:]), nil
}

// ReadInt64 reads a signed 64-bit scalar in Reader's default byte order.
func (reader *Reader) ReadInt64() (int64, error) {
	value, err := reader.ReadUint64()
	return int64(value), err
}

// ReadInt64WithOrder reads a signed 64-bit scalar using order rather than
// Reader's default byte order.
func (reader *Reader) ReadInt64WithOrder(order ByteOrder) (int64, error) {
	value, err := reader.ReadUint64WithOrder(order)
	return int64(value), err
}

// ReadFloat32 reads a 32-bit IEEE 754 floating-point value in Reader's default
// byte order.
func (reader *Reader) ReadFloat32() (float32, error) {
	value, err := reader.ReadUint32()
	return math.Float32frombits(value), err
}

// ReadFloat32WithOrder reads a 32-bit IEEE 754 floating-point value using
// order rather than Reader's default byte order.
func (reader *Reader) ReadFloat32WithOrder(order ByteOrder) (float32, error) {
	value, err := reader.ReadUint32WithOrder(order)
	return math.Float32frombits(value), err
}

// ReadFloat64 reads a 64-bit IEEE 754 floating-point value in Reader's default
// byte order.
func (reader *Reader) ReadFloat64() (float64, error) {
	value, err := reader.ReadUint64()
	return math.Float64frombits(value), err
}

// ReadFloat64WithOrder reads a 64-bit IEEE 754 floating-point value using
// order rather than Reader's default byte order.
func (reader *Reader) ReadFloat64WithOrder(order ByteOrder) (float64, error) {
	value, err := reader.ReadUint64WithOrder(order)
	return math.Float64frombits(value), err
}

func (reader *Reader) readable() error {
	if reader == nil || reader.in == nil {
		return ErrNilReader
	}
	return reader.initErr
}

func (reader *Reader) readBit() (bool, error) {
	if err := reader.readable(); err != nil {
		return false, err
	}
	if !reader.hasByte {
		if _, err := io.ReadFull(reader.in, reader.single[:]); err != nil {
			return false, err
		}
		reader.current = reader.single[0]
		reader.offset = 0
		reader.hasByte = true
	}

	shift := reader.offset
	if reader.order == MSBFirst {
		shift = 7 - reader.offset
	}
	value := reader.current&(1<<shift) != 0
	reader.offset++
	reader.position++
	if reader.offset == 8 {
		reader.clearCurrentByte()
	}
	return value, nil
}

func (reader *Reader) discardBytes(count uint64) error {
	var discard [4096]byte
	for count > 0 {
		chunkSize := len(discard)
		if count < uint64(chunkSize) {
			chunkSize = int(count)
		}
		read, err := io.ReadFull(reader.in, discard[:chunkSize])
		reader.position += uint64(read) * 8
		count -= uint64(read)
		if err != nil {
			return err
		}
	}
	return nil
}

func (reader *Reader) clearCurrentByte() {
	reader.current = 0
	reader.offset = 0
	reader.hasByte = false
}
