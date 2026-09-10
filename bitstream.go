// Package bitstream provides bit-level readers and writers over Go I/O streams.
//
// A Reader or Writer has a BitOrder that determines how bits are traversed
// within each byte and a default ByteOrder for fixed-width
// scalar values. Individual scalar operations can override that byte order.
package bitstream
