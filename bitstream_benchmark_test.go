package bitstream

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"testing"
)

const (
	benchmarkPayloadSize        = 64 * 1024
	benchmarkFieldsPerOperation = 8
)

func BenchmarkReadBits(b *testing.B) {
	data := bytes.Repeat([]byte{0xa5, 0x5a, 0x3c, 0xc3}, benchmarkPayloadSize/4)

	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		for _, width := range []uint8{1, 3, 8, 13, 32, 64} {
			for _, unaligned := range []bool{false, true} {
				for _, buffered := range []bool{false, true} {
					name := fmt.Sprintf("order=%s/width=%d/start=%s/source=%s",
						benchmarkOrderName(order), width, benchmarkAlignmentName(unaligned), benchmarkBufferName(buffered))
					b.Run(name, func(b *testing.B) {
						newReader := func() *Reader {
							source := bytes.NewReader(data)
							var input io.Reader = source
							if buffered {
								input = bufio.NewReaderSize(source, 32*1024)
							}
							reader := NewReader(input, WithBitOrder(order))
							if unaligned {
								if _, err := reader.ReadBits(3); err != nil {
									b.Fatalf("unaligned prefix: %v", err)
								}
							}
							return reader
						}

						reader := newReader()
						remaining := uint64(len(data)) * 8
						if unaligned {
							remaining -= 3
						}
						b.ReportAllocs()
						// Each reported operation processes eight fields, so width is
						// the exact number of payload bytes used for throughput.
						b.SetBytes(int64(width))
						b.ResetTimer()
						for index := 0; index < b.N; index++ {
							for field := 0; field < benchmarkFieldsPerOperation; field++ {
								if remaining < uint64(width) {
									reader = newReader()
									remaining = uint64(len(data)) * 8
									if unaligned {
										remaining -= 3
									}
								}
								if _, err := reader.ReadBits(width); err != nil {
									b.Fatalf("ReadBits(%d): %v", width, err)
								}
								remaining -= uint64(width)
							}
						}
					})
				}
			}
		}
	}
}

func BenchmarkWriteBits(b *testing.B) {
	for _, order := range []BitOrder{MSBFirst, LSBFirst} {
		for _, width := range []uint8{1, 3, 8, 13, 32, 64} {
			for _, unaligned := range []bool{false, true} {
				for _, buffered := range []bool{false, true} {
					name := fmt.Sprintf("order=%s/width=%d/start=%s/sink=%s",
						benchmarkOrderName(order), width, benchmarkAlignmentName(unaligned), benchmarkBufferName(buffered))
					b.Run(name, func(b *testing.B) {
						var output io.Writer = io.Discard
						var buffer *bufio.Writer
						if buffered {
							buffer = bufio.NewWriterSize(io.Discard, 32*1024)
							output = buffer
						}

						writer := NewWriter(output, WithBitOrder(order))
						if unaligned {
							if err := writer.WriteBits(0x5, 3); err != nil {
								b.Fatalf("unaligned prefix: %v", err)
							}
						}

						value := benchmarkValue(width)
						b.ReportAllocs()
						// Each reported operation processes eight fields, so width is
						// the exact number of payload bytes used for throughput.
						b.SetBytes(int64(width))
						b.ResetTimer()
						for index := 0; index < b.N; index++ {
							for field := 0; field < benchmarkFieldsPerOperation; field++ {
								if err := writer.WriteBits(value, width); err != nil {
									b.Fatal(err)
								}
							}
						}
						b.StopTimer()
						if err := writer.Close(); err != nil {
							b.Fatal(err)
						}
						if buffer != nil {
							if err := buffer.Flush(); err != nil {
								b.Fatal(err)
							}
						}
					})
				}
			}
		}
	}
}

func benchmarkValue(width uint8) uint64 {
	const value = uint64(0x9e3779b97f4a7c15)
	if width == 64 {
		return value
	}
	return value & (uint64(1)<<width - 1)
}

func benchmarkOrderName(order BitOrder) string {
	if order == MSBFirst {
		return "MSBFirst"
	}
	return "LSBFirst"
}

func benchmarkAlignmentName(unaligned bool) string {
	if unaligned {
		return "unaligned"
	}
	return "aligned"
}

func benchmarkBufferName(buffered bool) string {
	if buffered {
		return "buffered"
	}
	return "direct"
}
