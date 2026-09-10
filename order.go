package bitstream

import "encoding/binary"

// ByteOrder is an alias for encoding/binary.ByteOrder. It specifies the order
// of bytes in fixed-width scalar values.
type ByteOrder = binary.ByteOrder

var (
	// BigEndian is encoding/binary.BigEndian.
	BigEndian ByteOrder = binary.BigEndian

	// LittleEndian is encoding/binary.LittleEndian.
	LittleEndian ByteOrder = binary.LittleEndian
)

// BitOrder specifies how a bit stream traverses the bits in each byte.
type BitOrder uint8

const (
	// MSBFirst consumes or emits bit 7 before bit 0 in each byte. For a
	// multi-bit field, the first bit is the field's most-significant bit.
	MSBFirst BitOrder = iota

	// LSBFirst consumes or emits bit 0 before bit 7 in each byte. For a
	// multi-bit field, the first bit is the field's least-significant bit.
	LSBFirst
)

func (order BitOrder) valid() bool {
	return order == MSBFirst || order == LSBFirst
}

type config struct {
	bitOrder  BitOrder
	byteOrder ByteOrder
}

// Option configures a Reader or Writer.
type Option func(*config)

// WithBitOrder configures a Reader or Writer to use order. The default is
// MSBFirst.
func WithBitOrder(order BitOrder) Option {
	return func(config *config) {
		config.bitOrder = order
	}
}

// WithByteOrder configures the default byte order used by fixed-width scalar
// methods. The default is BigEndian. Per-field overrides are available
// through methods with a WithOrder suffix.
func WithByteOrder(order ByteOrder) Option {
	return func(config *config) {
		config.byteOrder = order
	}
}

func newConfig(options []Option) (config, error) {
	config := config{
		bitOrder:  MSBFirst,
		byteOrder: BigEndian,
	}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	if !config.bitOrder.valid() {
		return config, ErrInvalidBitOrder
	}
	if config.byteOrder == nil {
		return config, ErrNilByteOrder
	}
	return config, nil
}
