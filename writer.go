package bitstream

import (
	"io"
	"math"
)

// Writer writes a sequence of bits to an io.Writer.
//
// Writer is not safe for concurrent use. It implements io.Writer,
// io.ByteWriter, and io.Closer. Close pads a partial byte with zero bits and
// writes it, but never closes the underlying io.Writer.
type Writer struct {
	out       io.Writer
	order     BitOrder
	byteOrder ByteOrder
	current   byte
	bits      uint8
	position  uint64
	err       error
	closed    bool
	single    [1]byte
}

// NewWriter returns a Writer that writes to out. Its defaults are MSBFirst and
// BigEndian; configure them with WithBitOrder and WithByteOrder.
func NewWriter(out io.Writer, options ...Option) *Writer {
	config, err := newConfig(options)
	if err == nil && out == nil {
		err = ErrNilWriter
	}
	return &Writer{
		out:       out,
		order:     config.bitOrder,
		byteOrder: config.byteOrder,
		err:       err,
	}
}

// WriteBool writes one bit: one for true and zero for false.
func (writer *Writer) WriteBool(value bool) error {
	if err := writer.writable(); err != nil {
		return err
	}
	return writer.writeBit(value)
}

// WriteBits writes the low bitCount bits of value. bitCount must be between 1
// and 64, and value must fit in bitCount bits. Input validation happens before
// any stream state changes.
//
// With MSBFirst, bit bitCount-1 is emitted first. With LSBFirst, bit 0 is
// emitted first. If the underlying writer fails while a field is being written,
// some field bits may already have been emitted and Writer becomes unusable.
func (writer *Writer) WriteBits(value uint64, bitCount uint8) error {
	if err := writer.writable(); err != nil {
		return err
	}
	if bitCount == 0 || bitCount > 64 {
		return ErrInvalidBitCount
	}
	if bitCount < 64 && value>>bitCount != 0 {
		return ErrValueOverflow
	}

	if writer.order == MSBFirst {
		for index := bitCount; index > 0; index-- {
			if err := writer.writeBit(value&(uint64(1)<<(index-1)) != 0); err != nil {
				return err
			}
		}
		return nil
	}

	for index := range bitCount {
		if err := writer.writeBit(value&(uint64(1)<<index) != 0); err != nil {
			return err
		}
	}
	return nil
}

// WriteByte writes the next eight stream bits as value. It implements
// io.ByteWriter.
func (writer *Writer) WriteByte(value byte) error {
	return writer.WriteBits(uint64(value), 8)
}

// Write writes len(data) logical bytes to the bit stream. It implements
// io.Writer. At a byte boundary it writes directly to the underlying sink;
// otherwise it spreads each byte over the next eight stream bits.
func (writer *Writer) Write(data []byte) (int, error) {
	if err := writer.writable(); err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return 0, nil
	}

	if writer.bits == 0 {
		count, err := writer.out.Write(data)
		if count < 0 || count > len(data) {
			writer.err = errInvalidWrite
			return 0, writer.err
		}
		writer.position += uint64(count) * 8
		if err == nil && count != len(data) {
			err = io.ErrShortWrite
		}
		if err != nil {
			writer.err = err
		}
		return count, err
	}

	count := 0
	for count < len(data) {
		before := writer.position
		err := writer.WriteByte(data[count])
		if writer.position-before == 8 {
			count++
		}
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

// WriteString writes the raw bytes of value as logical bytes. It implements
// io.StringWriter.
func (writer *Writer) WriteString(value string) (int, error) {
	return writer.Write([]byte(value))
}

// PadToByte writes value until Writer reaches the next byte boundary. It
// returns the number of padding bits written. If Writer is byte-aligned, it
// returns zero.
func (writer *Writer) PadToByte(value bool) (uint8, error) {
	if err := writer.writable(); err != nil {
		return 0, err
	}
	var count uint8
	for writer.bits != 0 {
		count++
		if err := writer.writeBit(value); err != nil {
			return count, err
		}
	}
	return count, nil
}

// Align pads Writer to the next byte boundary with zero bits. It returns the
// number of padding bits written.
func (writer *Writer) Align() (uint8, error) {
	return writer.PadToByte(false)
}

// Close pads a partial byte with zero bits and writes it. Close is idempotent
// and does not close the underlying io.Writer.
func (writer *Writer) Close() error {
	if writer == nil {
		return ErrNilWriter
	}
	if writer.closed {
		return writer.err
	}
	if writer.err != nil {
		writer.closed = true
		return writer.err
	}
	_, err := writer.Align()
	writer.closed = true
	return err
}

// BitPosition reports the number of stream bits written, including padding
// added by Align or Close.
func (writer *Writer) BitPosition() uint64 {
	if writer == nil {
		return 0
	}
	return writer.position
}

// ByteAligned reports whether Writer is positioned at a byte boundary.
func (writer *Writer) ByteAligned() bool {
	return writer == nil || writer.bits == 0
}

// BitOrder reports Writer's configured bit order.
func (writer *Writer) BitOrder() BitOrder {
	if writer == nil {
		return MSBFirst
	}
	return writer.order
}

// ByteOrder reports Writer's default byte order for fixed-width scalar
// methods.
func (writer *Writer) ByteOrder() ByteOrder {
	if writer == nil {
		return BigEndian
	}
	return writer.byteOrder
}

// WriteUint8 writes one unsigned byte.
func (writer *Writer) WriteUint8(value uint8) error {
	return writer.WriteByte(byte(value))
}

// WriteInt8 writes one signed byte.
func (writer *Writer) WriteInt8(value int8) error {
	return writer.WriteByte(byte(value))
}

// WriteUint16 writes an unsigned 16-bit scalar in Writer's default byte order.
func (writer *Writer) WriteUint16(value uint16) error {
	if writer == nil {
		return ErrNilWriter
	}
	return writer.WriteUint16WithOrder(writer.byteOrder, value)
}

// WriteUint16WithOrder writes an unsigned 16-bit scalar using order rather
// than Writer's default byte order.
func (writer *Writer) WriteUint16WithOrder(order ByteOrder, value uint16) error {
	if order == nil {
		return ErrNilByteOrder
	}
	var data [2]byte
	order.PutUint16(data[:], value)
	_, err := writer.Write(data[:])
	return err
}

// WriteInt16 writes a signed 16-bit scalar in Writer's default byte order.
func (writer *Writer) WriteInt16(value int16) error {
	return writer.WriteUint16(uint16(value))
}

// WriteInt16WithOrder writes a signed 16-bit scalar using order rather than
// Writer's default byte order.
func (writer *Writer) WriteInt16WithOrder(order ByteOrder, value int16) error {
	return writer.WriteUint16WithOrder(order, uint16(value))
}

// WriteUint32 writes an unsigned 32-bit scalar in Writer's default byte order.
func (writer *Writer) WriteUint32(value uint32) error {
	if writer == nil {
		return ErrNilWriter
	}
	return writer.WriteUint32WithOrder(writer.byteOrder, value)
}

// WriteUint32WithOrder writes an unsigned 32-bit scalar using order rather
// than Writer's default byte order.
func (writer *Writer) WriteUint32WithOrder(order ByteOrder, value uint32) error {
	if order == nil {
		return ErrNilByteOrder
	}
	var data [4]byte
	order.PutUint32(data[:], value)
	_, err := writer.Write(data[:])
	return err
}

// WriteInt32 writes a signed 32-bit scalar in Writer's default byte order.
func (writer *Writer) WriteInt32(value int32) error {
	return writer.WriteUint32(uint32(value))
}

// WriteInt32WithOrder writes a signed 32-bit scalar using order rather than
// Writer's default byte order.
func (writer *Writer) WriteInt32WithOrder(order ByteOrder, value int32) error {
	return writer.WriteUint32WithOrder(order, uint32(value))
}

// WriteUint64 writes an unsigned 64-bit scalar in Writer's default byte order.
func (writer *Writer) WriteUint64(value uint64) error {
	if writer == nil {
		return ErrNilWriter
	}
	return writer.WriteUint64WithOrder(writer.byteOrder, value)
}

// WriteUint64WithOrder writes an unsigned 64-bit scalar using order rather
// than Writer's default byte order.
func (writer *Writer) WriteUint64WithOrder(order ByteOrder, value uint64) error {
	if order == nil {
		return ErrNilByteOrder
	}
	var data [8]byte
	order.PutUint64(data[:], value)
	_, err := writer.Write(data[:])
	return err
}

// WriteInt64 writes a signed 64-bit scalar in Writer's default byte order.
func (writer *Writer) WriteInt64(value int64) error {
	return writer.WriteUint64(uint64(value))
}

// WriteInt64WithOrder writes a signed 64-bit scalar using order rather than
// Writer's default byte order.
func (writer *Writer) WriteInt64WithOrder(order ByteOrder, value int64) error {
	return writer.WriteUint64WithOrder(order, uint64(value))
}

// WriteFloat32 writes a 32-bit IEEE 754 floating-point value in Writer's
// default byte order.
func (writer *Writer) WriteFloat32(value float32) error {
	return writer.WriteUint32(math.Float32bits(value))
}

// WriteFloat32WithOrder writes a 32-bit IEEE 754 floating-point value using
// order rather than Writer's default byte order.
func (writer *Writer) WriteFloat32WithOrder(order ByteOrder, value float32) error {
	return writer.WriteUint32WithOrder(order, math.Float32bits(value))
}

// WriteFloat64 writes a 64-bit IEEE 754 floating-point value in Writer's
// default byte order.
func (writer *Writer) WriteFloat64(value float64) error {
	return writer.WriteUint64(math.Float64bits(value))
}

// WriteFloat64WithOrder writes a 64-bit IEEE 754 floating-point value using
// order rather than Writer's default byte order.
func (writer *Writer) WriteFloat64WithOrder(order ByteOrder, value float64) error {
	return writer.WriteUint64WithOrder(order, math.Float64bits(value))
}

func (writer *Writer) writable() error {
	if writer == nil || writer.out == nil {
		return ErrNilWriter
	}
	if writer.err != nil {
		return writer.err
	}
	if writer.closed {
		return ErrClosed
	}
	return nil
}

func (writer *Writer) writeBit(value bool) error {
	shift := writer.bits
	if writer.order == MSBFirst {
		shift = 7 - writer.bits
	}
	if value {
		writer.current |= 1 << shift
	}
	writer.bits++
	writer.position++
	if writer.bits == 8 {
		return writer.flushCurrentByte()
	}
	return nil
}

func (writer *Writer) flushCurrentByte() error {
	writer.single[0] = writer.current
	count, err := writer.out.Write(writer.single[:])
	if count < 0 || count > 1 {
		writer.err = errInvalidWrite
		return writer.err
	}
	if count == 1 {
		writer.current = 0
		writer.bits = 0
	}
	if err == nil && count != 1 {
		err = io.ErrShortWrite
	}
	if err != nil {
		writer.err = err
	}
	return err
}
