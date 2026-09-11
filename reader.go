package bitstream

import (
	"bytes"
	"io"
	"io/fs"
	"math"
)

// Reader reads a sequence of bits from an io.Reader.
//
// Reader is not safe for concurrent use. It implements io.Reader and
// io.ByteReader. Its byte-oriented methods assemble the next eight stream
// bits into each returned byte, so they work at non-byte-aligned positions.
// When NewReader detects a bounded io.ReaderAt source, Reader uses ReadAt and
// supports forks and peeks without changing the source's seek position.
type Reader struct {
	in        io.Reader
	at        io.ReaderAt
	order     BitOrder
	byteOrder ByteOrder
	current   byte
	offset    uint8
	hasByte   bool
	position  uint64
	limit     uint64
	origin    int64
	random    bool
	initErr   error
	single    [1]byte
}

type sizedReader interface {
	Size() int64
}

type statReader interface {
	Stat() (fs.FileInfo, error)
}

// NewReader returns a Reader that reads from in. Its defaults are MSBFirst and
// BigEndian; configure them with WithBitOrder and WithByteOrder.
func NewReader(in io.Reader, options ...Option) *Reader {
	config, err := newConfig(options)
	if err == nil && in == nil {
		err = ErrNilReader
	}
	reader := &Reader{
		in:        in,
		order:     config.bitOrder,
		byteOrder: config.byteOrder,
		initErr:   err,
	}
	if err == nil {
		reader.initErr = reader.detectRandomAccess()
	}
	return reader
}

// NewReaderFromBytes returns a Reader over data. The data is not copied.
func NewReaderFromBytes(data []byte, options ...Option) *Reader {
	return NewReader(bytes.NewReader(data), options...)
}

// CanFork reports whether Fork is available. It returns false for nil,
// unsuccessfully initialized, and forward-only Readers. Calling CanFork does
// not consume input or otherwise change Reader's state.
func (reader *Reader) CanFork() bool {
	return reader != nil && reader.in != nil && reader.at != nil && reader.initErr == nil && reader.random
}

// CanPeek reports whether PeekBits is available. It has the same result as
// CanFork and does not consume input or otherwise change Reader's state.
func (reader *Reader) CanPeek() bool {
	return reader.CanFork()
}

// Fork returns an independent Reader at the current bit position. Forks are
// available only for bounded random-access sources. The source data must remain
// unchanged while any Reader using it is active.
func (reader *Reader) Fork() (*Reader, error) {
	if err := reader.readable(); err != nil {
		return nil, err
	}
	if !reader.random {
		return nil, ErrRandomAccessUnavailable
	}
	fork := *reader
	return &fork, nil
}

// ForkAndSkip returns a child limited to the next byteCount logical bytes and
// advances Reader past those bytes. The operation is atomic: on error Reader is
// unchanged and no child is returned.
func (reader *Reader) ForkAndSkip(byteCount uint64) (*Reader, error) {
	if err := reader.readable(); err != nil {
		return nil, err
	}
	if !reader.random {
		return nil, ErrRandomAccessUnavailable
	}
	if byteCount > ^uint64(0)/8 {
		return nil, ErrBitCountOverflow
	}
	bitCount := byteCount * 8
	if bitCount > reader.limit-reader.position {
		return nil, io.ErrUnexpectedEOF
	}

	fork := *reader
	fork.limit = reader.position + bitCount
	reader.advanceRandom(bitCount)
	return &fork, nil
}

// PeekBits reads bitCount bits without changing Reader. It is available only
// for bounded random-access sources.
func (reader *Reader) PeekBits(bitCount uint8) (uint64, error) {
	if err := reader.readable(); err != nil {
		return 0, err
	}
	if bitCount == 0 || bitCount > 64 {
		return 0, ErrInvalidBitCount
	}
	if !reader.random {
		return 0, ErrRandomAccessUnavailable
	}
	peek := *reader
	return peek.ReadBits(bitCount)
}

// ReadBool reads one bit and returns true for a set bit.
func (reader *Reader) ReadBool() (bool, error) {
	return reader.readBit()
}

// ReadBits reads bitCount bits and returns them in the low bitCount bits of the
// result. bitCount must be between 1 and 64.
//
// With MSBFirst, the first stream bit becomes bit bitCount-1 of the result. With
// LSBFirst, it becomes bit 0. If EOF occurs before any requested bit is read,
// ReadBits returns io.EOF. If EOF occurs after one or more requested bits have
// been consumed, it returns io.ErrUnexpectedEOF. Other underlying errors are
// returned unchanged. On any read error, Reader remains advanced by the bits
// already consumed and ReadBits returns zero.
func (reader *Reader) ReadBits(bitCount uint8) (uint64, error) {
	if err := reader.readable(); err != nil {
		return 0, err
	}
	if bitCount == 0 || bitCount > 64 {
		return 0, ErrInvalidBitCount
	}

	var value uint64
	if reader.order == MSBFirst {
		for index := range bitCount {
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

	for index := range bitCount {
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

func readBitsError(err error, consumedBits uint8) error {
	if consumedBits > 0 && err == io.EOF {
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

	if !reader.hasByte && reader.position%8 == 0 {
		if reader.random {
			return reader.readRandom(data)
		}
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

// ReadBitsToSlice reads bitCount bits into a newly allocated packed byte slice.
// It returns ceil(bitCount / 8) bytes. Each complete group of eight bits is a
// logical byte; when bitCount is not a multiple of eight, the final byte
// contains the remaining field value in its low bits according to ReadBits
// semantics.
//
// If the stream ends first, it returns the completed packed bytes and the read
// error. A final partial group is omitted when it cannot be read completely.
// bitCount must produce a slice length that fits in a Go int.
//
// This method allocates the requested result before reading. Callers must
// validate input-derived bit counts against an application-specific allocation
// limit before calling it.
func (reader *Reader) ReadBitsToSlice(bitCount uint64) ([]byte, error) {
	if err := reader.readable(); err != nil {
		return nil, err
	}

	length := bitCount / 8
	if bitCount%8 != 0 {
		length++
	}
	if length > uint64(^uint(0)>>1) {
		return nil, ErrSliceLengthOverflow
	}

	data := make([]byte, int(length))
	fullBytes := int(bitCount / 8)
	for index := range fullBytes {
		value, err := reader.ReadByte()
		if err != nil {
			return data[:index], err
		}
		data[index] = value
	}

	if remaining := uint8(bitCount % 8); remaining != 0 {
		value, err := reader.ReadBits(remaining)
		if err != nil {
			return data[:fullBytes], err
		}
		data[fullBytes] = byte(value)
	}
	return data, nil
}

// ReadBytesToSlice reads byteCount logical bytes into a newly allocated slice.
// If the stream ends first, it returns the bytes read and the read error.
// byteCount must fit in a Go slice length.
//
// This method allocates the requested result before reading. Callers must
// validate input-derived byte counts against an application-specific allocation
// limit before calling it.
func (reader *Reader) ReadBytesToSlice(byteCount uint64) ([]byte, error) {
	if err := reader.readable(); err != nil {
		return nil, err
	}
	if byteCount > uint64(^uint(0)>>1) {
		return nil, ErrSliceLengthOverflow
	}

	data := make([]byte, int(byteCount))
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
	for index := range length {
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

// SkipBits consumes bitCount bits without returning them. If an I/O error occurs,
// Reader remains advanced by every bit consumed before that error.
func (reader *Reader) SkipBits(bitCount uint64) error {
	if err := reader.readable(); err != nil {
		return err
	}
	if reader.random {
		return reader.skipRandomBits(bitCount)
	}

	if reader.hasByte && bitCount > 0 {
		remaining := uint64(8 - reader.offset)
		consumed := min(bitCount, remaining)
		reader.offset += uint8(consumed)
		reader.position += consumed
		bitCount -= consumed
		if reader.offset == 8 {
			reader.clearCurrentByte()
		}
	}

	bytesToSkip := bitCount / 8
	if bytesToSkip > 0 {
		if err := reader.discardBytes(bytesToSkip); err != nil {
			return err
		}
		bitCount -= bytesToSkip * 8
	}

	for range bitCount {
		if _, err := reader.readBit(); err != nil {
			return err
		}
	}
	return nil
}

// SkipBytes consumes byteCount logical bytes without returning them.
func (reader *Reader) SkipBytes(byteCount uint64) error {
	const maxUint64 = ^uint64(0)
	if byteCount > maxUint64/8 {
		return ErrBitCountOverflow
	}
	return reader.SkipBits(byteCount * 8)
}

// Align discards the unread bits in the current byte and returns their count.
// If Reader is already byte-aligned, it returns zero.
func (reader *Reader) Align() uint8 {
	if reader == nil || reader.position%8 == 0 {
		return 0
	}
	if reader.random {
		if reader.position >= reader.limit {
			reader.clearCurrentByte()
			return 0
		}
		toBoundary := uint64(8 - reader.position%8)
		skipped := min(toBoundary, reader.limit-reader.position)
		reader.position += skipped
		reader.clearCurrentByte()
		return uint8(skipped)
	}
	if !reader.hasByte {
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
// consuming input. Bounded random-access sources use their captured bounds;
// other seekable sources are queried without consuming input.
func (reader *Reader) BitsRemaining() (uint64, error) {
	if err := reader.readable(); err != nil {
		return 0, err
	}
	if reader.random {
		return reader.limit - reader.position, nil
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
	return reader.bitsRemainingFromBytes(uint64(end - current))
}

func (reader *Reader) bitsRemainingFromBytes(remainingBytes uint64) (uint64, error) {
	if remainingBytes > ^uint64(0)/8 {
		return 0, ErrBitCountOverflow
	}
	remaining := remainingBytes * 8
	if reader.hasByte {
		buffered := uint64(8 - reader.offset)
		if remaining > ^uint64(0)-buffered {
			return 0, ErrBitCountOverflow
		}
		remaining += buffered
	}
	return remaining, nil
}

// ByteAligned reports whether Reader is positioned at a byte boundary.
func (reader *Reader) ByteAligned() bool {
	return reader == nil || reader.position%8 == 0
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
	if reader.random && reader.position >= reader.limit {
		return false, io.EOF
	}
	if !reader.hasByte {
		if reader.random {
			count, err := reader.at.ReadAt(reader.single[:], reader.origin+int64(reader.position/8))
			if count < 0 || count > len(reader.single) {
				return false, errInvalidRead
			}
			if count != len(reader.single) {
				if err == nil {
					err = io.ErrUnexpectedEOF
				}
				return false, err
			}
		} else if _, err := io.ReadFull(reader.in, reader.single[:]); err != nil {
			return false, err
		}
		reader.current = reader.single[0]
		reader.offset = uint8(reader.position % 8)
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

func (reader *Reader) readRandom(data []byte) (int, error) {
	if reader.position >= reader.limit {
		return 0, io.EOF
	}

	availableBytes := (reader.limit - reader.position) / 8
	if availableBytes == 0 {
		_, err := reader.ReadByte()
		return 0, err
	}
	count := len(data)
	if uint64(count) > availableBytes {
		count = int(availableBytes)
	}

	read, err := reader.at.ReadAt(data[:count], reader.origin+int64(reader.position/8))
	if read < 0 || read > count {
		return 0, errInvalidRead
	}
	reader.position += uint64(read) * 8
	return read, err
}

func (reader *Reader) skipRandomBits(bitCount uint64) error {
	if bitCount == 0 {
		return nil
	}
	if reader.position >= reader.limit {
		return io.EOF
	}

	consumed := min(bitCount, reader.limit-reader.position)
	reader.advanceRandom(consumed)
	if consumed == bitCount {
		return nil
	}
	if consumed == 0 {
		return io.EOF
	}
	return io.ErrUnexpectedEOF
}

func (reader *Reader) advanceRandom(bitCount uint64) {
	if bitCount == 0 {
		return
	}
	next := reader.position + bitCount
	if reader.hasByte {
		byteStart := reader.position - uint64(reader.offset)
		if next > byteStart && next < byteStart+8 {
			reader.offset = uint8(next - byteStart)
		} else {
			reader.clearCurrentByte()
		}
	}
	reader.position = next
}

func (reader *Reader) discardBytes(byteCount uint64) error {
	var discard [4096]byte
	for byteCount > 0 {
		chunkSize := len(discard)
		if byteCount < uint64(chunkSize) {
			chunkSize = int(byteCount)
		}
		read, err := io.ReadFull(reader.in, discard[:chunkSize])
		reader.position += uint64(read) * 8
		byteCount -= uint64(read)
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

func (reader *Reader) detectRandomAccess() error {
	at, ok := reader.in.(io.ReaderAt)
	if !ok {
		return nil
	}

	var size int64
	if source, ok := reader.in.(sizedReader); ok {
		size = source.Size()
	} else {
		source, ok := reader.in.(statReader)
		if !ok {
			return nil
		}
		info, err := source.Stat()
		if err != nil {
			return err
		}
		if info == nil {
			return ErrInvalidSize
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		size = info.Size()
	}
	if size < 0 {
		return ErrInvalidSize
	}

	origin := int64(0)
	if seeker, ok := reader.in.(io.Seeker); ok {
		current, err := seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		origin = current
	}
	if origin < 0 || origin > size {
		return ErrInvalidSize
	}

	available := uint64(size - origin)
	if available > ^uint64(0)/8 {
		return ErrBitCountOverflow
	}
	reader.at = at
	reader.origin = origin
	reader.limit = available * 8
	reader.random = true
	return nil
}
