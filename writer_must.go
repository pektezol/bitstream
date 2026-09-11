package bitstream

// MustWriteBool writes one bit and panics if it cannot be written.
func (writer *Writer) MustWriteBool(value bool) {
	if err := writer.WriteBool(value); err != nil {
		panic(err)
	}
}

// MustWriteBits writes bitCount bits and panics if they cannot be written.
func (writer *Writer) MustWriteBits(value uint64, bitCount uint8) {
	if err := writer.WriteBits(value, bitCount); err != nil {
		panic(err)
	}
}

// MustWriteByte writes one byte and panics if it cannot be written.
func (writer *Writer) MustWriteByte(value byte) {
	if err := writer.WriteByte(value); err != nil {
		panic(err)
	}
}

// MustWrite writes data and panics if Write returns an error. It returns the
// number of logical bytes written.
func (writer *Writer) MustWrite(data []byte) int {
	count, err := writer.Write(data)
	if err != nil {
		panic(err)
	}
	return count
}

// MustWriteString writes the raw bytes of value and panics if they cannot be
// written. It returns the number of logical bytes written.
func (writer *Writer) MustWriteString(value string) int {
	count, err := writer.WriteString(value)
	if err != nil {
		panic(err)
	}
	return count
}

// MustPadToByte writes value until Writer reaches a byte boundary and panics if
// the padding cannot be written. It returns the number of padding bits written.
func (writer *Writer) MustPadToByte(value bool) uint8 {
	count, err := writer.PadToByte(value)
	if err != nil {
		panic(err)
	}
	return count
}

// MustAlign pads Writer to a byte boundary with zero bits and panics if the
// padding cannot be written. It returns the number of padding bits written.
func (writer *Writer) MustAlign() uint8 {
	count, err := writer.Align()
	if err != nil {
		panic(err)
	}
	return count
}

// MustClose finalizes Writer and panics if it cannot be closed.
func (writer *Writer) MustClose() {
	if err := writer.Close(); err != nil {
		panic(err)
	}
}

// MustWriteUint8 writes one unsigned byte and panics if it cannot be written.
func (writer *Writer) MustWriteUint8(value uint8) {
	if err := writer.WriteUint8(value); err != nil {
		panic(err)
	}
}

// MustWriteInt8 writes one signed byte and panics if it cannot be written.
func (writer *Writer) MustWriteInt8(value int8) {
	if err := writer.WriteInt8(value); err != nil {
		panic(err)
	}
}

// MustWriteUint16 writes an unsigned 16-bit scalar in Writer's default byte
// order and panics if it cannot be written.
func (writer *Writer) MustWriteUint16(value uint16) {
	if err := writer.WriteUint16(value); err != nil {
		panic(err)
	}
}

// MustWriteUint16WithOrder writes an unsigned 16-bit scalar using order rather
// than Writer's default byte order and panics if it cannot be written.
func (writer *Writer) MustWriteUint16WithOrder(order ByteOrder, value uint16) {
	if err := writer.WriteUint16WithOrder(order, value); err != nil {
		panic(err)
	}
}

// MustWriteInt16 writes a signed 16-bit scalar in Writer's default byte order
// and panics if it cannot be written.
func (writer *Writer) MustWriteInt16(value int16) {
	if err := writer.WriteInt16(value); err != nil {
		panic(err)
	}
}

// MustWriteInt16WithOrder writes a signed 16-bit scalar using order rather
// than Writer's default byte order and panics if it cannot be written.
func (writer *Writer) MustWriteInt16WithOrder(order ByteOrder, value int16) {
	if err := writer.WriteInt16WithOrder(order, value); err != nil {
		panic(err)
	}
}

// MustWriteUint32 writes an unsigned 32-bit scalar in Writer's default byte
// order and panics if it cannot be written.
func (writer *Writer) MustWriteUint32(value uint32) {
	if err := writer.WriteUint32(value); err != nil {
		panic(err)
	}
}

// MustWriteUint32WithOrder writes an unsigned 32-bit scalar using order rather
// than Writer's default byte order and panics if it cannot be written.
func (writer *Writer) MustWriteUint32WithOrder(order ByteOrder, value uint32) {
	if err := writer.WriteUint32WithOrder(order, value); err != nil {
		panic(err)
	}
}

// MustWriteInt32 writes a signed 32-bit scalar in Writer's default byte order
// and panics if it cannot be written.
func (writer *Writer) MustWriteInt32(value int32) {
	if err := writer.WriteInt32(value); err != nil {
		panic(err)
	}
}

// MustWriteInt32WithOrder writes a signed 32-bit scalar using order rather
// than Writer's default byte order and panics if it cannot be written.
func (writer *Writer) MustWriteInt32WithOrder(order ByteOrder, value int32) {
	if err := writer.WriteInt32WithOrder(order, value); err != nil {
		panic(err)
	}
}

// MustWriteUint64 writes an unsigned 64-bit scalar in Writer's default byte
// order and panics if it cannot be written.
func (writer *Writer) MustWriteUint64(value uint64) {
	if err := writer.WriteUint64(value); err != nil {
		panic(err)
	}
}

// MustWriteUint64WithOrder writes an unsigned 64-bit scalar using order rather
// than Writer's default byte order and panics if it cannot be written.
func (writer *Writer) MustWriteUint64WithOrder(order ByteOrder, value uint64) {
	if err := writer.WriteUint64WithOrder(order, value); err != nil {
		panic(err)
	}
}

// MustWriteInt64 writes a signed 64-bit scalar in Writer's default byte order
// and panics if it cannot be written.
func (writer *Writer) MustWriteInt64(value int64) {
	if err := writer.WriteInt64(value); err != nil {
		panic(err)
	}
}

// MustWriteInt64WithOrder writes a signed 64-bit scalar using order rather
// than Writer's default byte order and panics if it cannot be written.
func (writer *Writer) MustWriteInt64WithOrder(order ByteOrder, value int64) {
	if err := writer.WriteInt64WithOrder(order, value); err != nil {
		panic(err)
	}
}

// MustWriteFloat32 writes a 32-bit IEEE 754 floating-point value in Writer's
// default byte order and panics if it cannot be written.
func (writer *Writer) MustWriteFloat32(value float32) {
	if err := writer.WriteFloat32(value); err != nil {
		panic(err)
	}
}

// MustWriteFloat32WithOrder writes a 32-bit IEEE 754 floating-point value using
// order rather than Writer's default byte order and panics if it cannot be
// written.
func (writer *Writer) MustWriteFloat32WithOrder(order ByteOrder, value float32) {
	if err := writer.WriteFloat32WithOrder(order, value); err != nil {
		panic(err)
	}
}

// MustWriteFloat64 writes a 64-bit IEEE 754 floating-point value in Writer's
// default byte order and panics if it cannot be written.
func (writer *Writer) MustWriteFloat64(value float64) {
	if err := writer.WriteFloat64(value); err != nil {
		panic(err)
	}
}

// MustWriteFloat64WithOrder writes a 64-bit IEEE 754 floating-point value using
// order rather than Writer's default byte order and panics if it cannot be
// written.
func (writer *Writer) MustWriteFloat64WithOrder(order ByteOrder, value float64) {
	if err := writer.WriteFloat64WithOrder(order, value); err != nil {
		panic(err)
	}
}
