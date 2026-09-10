package bitstream

// MustReadBool reads one bit and panics if it cannot be read.
func (reader *Reader) MustReadBool() bool {
	value, err := reader.ReadBool()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadBits reads count bits and panics if they cannot be read.
func (reader *Reader) MustReadBits(count uint8) uint64 {
	value, err := reader.ReadBits(count)
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadByte reads one byte and panics if it cannot be read.
func (reader *Reader) MustReadByte() byte {
	value, err := reader.ReadByte()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadBitsToSlice reads bits into a packed byte slice and panics if they
// cannot be read.
func (reader *Reader) MustReadBitsToSlice(bits uint64) []byte {
	value, err := reader.ReadBitsToSlice(bits)
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadBytesToSlice reads count logical bytes into a newly allocated slice
// and panics if they cannot be read.
func (reader *Reader) MustReadBytesToSlice(count uint64) []byte {
	value, err := reader.ReadBytesToSlice(count)
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadStringToNull reads logical bytes through a null byte and panics if it
// cannot be read. The returned string excludes the null byte.
func (reader *Reader) MustReadStringToNull() string {
	value, err := reader.ReadStringToNull()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadStringToLength reads length logical bytes and panics if they cannot
// be read.
func (reader *Reader) MustReadStringToLength(length uint64) string {
	value, err := reader.ReadStringToLength(length)
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadUint8 reads one unsigned byte and panics if it cannot be read.
func (reader *Reader) MustReadUint8() uint8 {
	value, err := reader.ReadUint8()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadInt8 reads one signed byte and panics if it cannot be read.
func (reader *Reader) MustReadInt8() int8 {
	value, err := reader.ReadInt8()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadUint16 reads an unsigned 16-bit scalar in Reader's default byte order
// and panics if it cannot be read.
func (reader *Reader) MustReadUint16() uint16 {
	value, err := reader.ReadUint16()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadUint16WithOrder reads an unsigned 16-bit scalar using order rather
// than Reader's default byte order and panics if it cannot be read.
func (reader *Reader) MustReadUint16WithOrder(order ByteOrder) uint16 {
	value, err := reader.ReadUint16WithOrder(order)
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadInt16 reads a signed 16-bit scalar in Reader's default byte order and
// panics if it cannot be read.
func (reader *Reader) MustReadInt16() int16 {
	value, err := reader.ReadInt16()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadInt16WithOrder reads a signed 16-bit scalar using order rather than
// Reader's default byte order and panics if it cannot be read.
func (reader *Reader) MustReadInt16WithOrder(order ByteOrder) int16 {
	value, err := reader.ReadInt16WithOrder(order)
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadUint32 reads an unsigned 32-bit scalar in Reader's default byte order
// and panics if it cannot be read.
func (reader *Reader) MustReadUint32() uint32 {
	value, err := reader.ReadUint32()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadUint32WithOrder reads an unsigned 32-bit scalar using order rather
// than Reader's default byte order and panics if it cannot be read.
func (reader *Reader) MustReadUint32WithOrder(order ByteOrder) uint32 {
	value, err := reader.ReadUint32WithOrder(order)
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadInt32 reads a signed 32-bit scalar in Reader's default byte order and
// panics if it cannot be read.
func (reader *Reader) MustReadInt32() int32 {
	value, err := reader.ReadInt32()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadInt32WithOrder reads a signed 32-bit scalar using order rather than
// Reader's default byte order and panics if it cannot be read.
func (reader *Reader) MustReadInt32WithOrder(order ByteOrder) int32 {
	value, err := reader.ReadInt32WithOrder(order)
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadUint64 reads an unsigned 64-bit scalar in Reader's default byte order
// and panics if it cannot be read.
func (reader *Reader) MustReadUint64() uint64 {
	value, err := reader.ReadUint64()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadUint64WithOrder reads an unsigned 64-bit scalar using order rather
// than Reader's default byte order and panics if it cannot be read.
func (reader *Reader) MustReadUint64WithOrder(order ByteOrder) uint64 {
	value, err := reader.ReadUint64WithOrder(order)
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadInt64 reads a signed 64-bit scalar in Reader's default byte order and
// panics if it cannot be read.
func (reader *Reader) MustReadInt64() int64 {
	value, err := reader.ReadInt64()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadInt64WithOrder reads a signed 64-bit scalar using order rather than
// Reader's default byte order and panics if it cannot be read.
func (reader *Reader) MustReadInt64WithOrder(order ByteOrder) int64 {
	value, err := reader.ReadInt64WithOrder(order)
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadFloat32 reads a 32-bit IEEE 754 floating-point value in Reader's
// default byte order and panics if it cannot be read.
func (reader *Reader) MustReadFloat32() float32 {
	value, err := reader.ReadFloat32()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadFloat32WithOrder reads a 32-bit IEEE 754 floating-point value using
// order rather than Reader's default byte order and panics if it cannot be read.
func (reader *Reader) MustReadFloat32WithOrder(order ByteOrder) float32 {
	value, err := reader.ReadFloat32WithOrder(order)
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadFloat64 reads a 64-bit IEEE 754 floating-point value in Reader's
// default byte order and panics if it cannot be read.
func (reader *Reader) MustReadFloat64() float64 {
	value, err := reader.ReadFloat64()
	if err != nil {
		panic(err)
	}
	return value
}

// MustReadFloat64WithOrder reads a 64-bit IEEE 754 floating-point value using
// order rather than Reader's default byte order and panics if it cannot be read.
func (reader *Reader) MustReadFloat64WithOrder(order ByteOrder) float64 {
	value, err := reader.ReadFloat64WithOrder(order)
	if err != nil {
		panic(err)
	}
	return value
}
