package bitstream

import "errors"

var (
	// ErrInvalidBitOrder is returned when a Reader or Writer is configured with
	// a BitOrder other than MSBFirst or LSBFirst.
	ErrInvalidBitOrder = errors.New("bitstream: invalid bit order")

	// ErrInvalidBitCount is returned when a bit operation requests fewer than
	// one or more than 64 bits.
	ErrInvalidBitCount = errors.New("bitstream: bit count must be between 1 and 64")

	// ErrValueOverflow is returned when WriteBits receives a value that cannot
	// be represented by the requested number of bits.
	ErrValueOverflow = errors.New("bitstream: value does not fit in bit count")

	// ErrBitCountOverflow is returned when converting a byte count to a bit
	// count would overflow uint64.
	ErrBitCountOverflow = errors.New("bitstream: bit count overflows uint64")

	// ErrStringLengthOverflow is returned when a requested string length cannot
	// be represented by a Go slice length.
	ErrStringLengthOverflow = errors.New("bitstream: string length overflows int")

	// ErrSliceLengthOverflow is returned when a requested byte or packed-bit
	// slice length cannot be represented by a Go slice length.
	ErrSliceLengthOverflow = errors.New("bitstream: slice length overflows int")

	// ErrRemainingBitsUnavailable is returned when the source of a Reader does
	// not support seeking, so its distance to EOF cannot be determined without
	// consuming input.
	ErrRemainingBitsUnavailable = errors.New("bitstream: remaining bits unavailable")

	// ErrClosed is returned when writing to a Writer after Close.
	ErrClosed = errors.New("bitstream: writer is closed")

	// ErrNilReader is returned when a Reader is constructed with a nil source.
	ErrNilReader = errors.New("bitstream: nil reader")

	// ErrNilWriter is returned when a Writer is constructed with a nil sink.
	ErrNilWriter = errors.New("bitstream: nil writer")

	// ErrNilByteOrder is returned when a Reader or Writer is configured with a
	// nil ByteOrder, or when a scalar override receives one.
	ErrNilByteOrder = errors.New("bitstream: nil byte order")
)

var (
	errInvalidRead  = errors.New("bitstream: invalid read count")
	errInvalidWrite = errors.New("bitstream: invalid write count")
)
