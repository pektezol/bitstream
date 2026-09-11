# bitstream

`bitstream` is a small Go library for reading and writing packed binary data.
It provides independent bit order and byte order controls:

- `BitOrder` describes the order of bits within each byte.
- `ByteOrder` describes the order of bytes in fixed-width scalar values; it is
  an alias for `encoding/binary.ByteOrder`.

The default bit order is `MSBFirst`; the default byte order for fixed-width
scalars is `BigEndian`.

## Example

```go
var output bytes.Buffer

w := bitstream.NewWriter(&output,
    bitstream.WithBitOrder(bitstream.MSBFirst),
    bitstream.WithByteOrder(bitstream.LittleEndian),
)
if err := w.WriteBits(0x5, 3); err != nil { /* handle error */ }
if err := w.WriteUint16(0x1234); err != nil { /* handle error */ }
if err := w.Close(); err != nil { /* handle error */ }

r := bitstream.NewReaderFromBytes(output.Bytes(),
    bitstream.WithBitOrder(bitstream.MSBFirst),
    bitstream.WithByteOrder(bitstream.LittleEndian),
)
flags, err := r.ReadBits(3)                         // 0x5
length, err := r.ReadUint16()                        // 0x1234
```

`Close` pads an unfinished byte with zero bits and writes it. It does not close
the underlying `io.Writer`.

## Bit order

With `MSBFirst`, a byte is traversed from bit 7 to bit 0, and the first bit of
a field becomes that field's most-significant bit. With `LSBFirst`, a byte is
traversed from bit 0 to bit 7, and the first bit becomes the field's
least-significant bit.

Use `WithByteOrder` to establish a format-wide default for multi-byte scalar
helpers. A method such as `ReadUint32()` uses that default; an exceptional
field can use `ReadUint32WithOrder(LittleEndian)`. Writer methods follow the
same pattern. Byte order remains independent of the configured bit order.

## API boundaries

- `ReadBits` and `WriteBits` operate on fields from 1 to 64 bits.
- `WriteBits` rejects values that cannot fit in the requested field width.
- Reader bit, slice, string, and scalar operations have `MustRead...`
  counterparts; every error-returning writer operation has a `MustWrite...`
  counterpart. They return values when applicable or panic with the original
  error.
- `Reader` implements `io.Reader` and `io.ByteReader`; `Writer` implements
  `io.Writer`, `io.ByteWriter`, and `io.Closer`.
- Byte-oriented reads and writes also work from an unaligned bit position; the
	 next eight stream bits form each logical byte.
- String helpers operate on raw string bytes; they do not validate UTF-8.
- `Reader.Align` discards the remaining bits of its current byte. `Writer.Align`
  pads the current byte with zero bits. `Writer.PadToByte` allows one-bit
  padding instead.
- `Reader` and `Writer` are not safe for concurrent use.

## Full API

The method signatures below omit their `Reader` or `Writer` receiver.

### Configuration

`Option` configures a `Reader` or `Writer` during construction.

```go
type ByteOrder = binary.ByteOrder

var (
    BigEndian ByteOrder
    LittleEndian ByteOrder
)

type BitOrder uint8

const (
    MSBFirst BitOrder = iota
    LSBFirst
)

WithBitOrder(order BitOrder) Option
WithByteOrder(order ByteOrder) Option
```

`MSBFirst` consumes or emits bit 7 before bit 0 in each byte. `LSBFirst`
consumes or emits bit 0 before bit 7. The default bit order is `MSBFirst`; the
default byte order is `BigEndian`. `BigEndian` and `LittleEndian` are
convenience values equivalent to their `encoding/binary` counterparts.

### Reader

`Reader` implements `io.Reader` and `io.ByteReader`.

```go
NewReader(in io.Reader, options ...Option) *Reader
NewReaderFromBytes(data []byte, options ...Option) *Reader

CanFork() bool
CanPeek() bool
Fork() (*Reader, error)
ForkAndSkip(byteCount uint64) (*Reader, error)
PeekBits(bitCount uint8) (uint64, error)
MustPeekBits(bitCount uint8) uint64

BitPosition() uint64
BitsRemaining() (uint64, error)
ByteAligned() bool
BitOrder() BitOrder
ByteOrder() ByteOrder
```

`NewReaderFromBytes(data, options...)` is a convenience wrapper around
`NewReader(bytes.NewReader(data), options...)`; it retains `data` rather than
copying it. `BitPosition` includes bits discarded by `Align`.

`NewReader` detects bounded random access when its source implements both
`io.ReaderAt` and either `Size() int64` or `Stat()` for a regular file.
`bytes.Reader`, `strings.Reader`, `io.SectionReader`, and regular files qualify
automatically. A custom source can participate by exposing `Size() int64`.
When the source is also an `io.Seeker`, its current offset becomes the reader's
origin without changing that offset. The captured extent does not grow if the
backing file grows, and the backing data must remain unchanged while the reader
is in use.

Bounded sources support independent `Fork` readers, bounded `ForkAndSkip`
children, and non-consuming `PeekBits`. `ForkAndSkip(n)` limits the child to
the next `n` logical bytes and advances the parent past them; if that extent is
unavailable, it returns `io.ErrUnexpectedEOF` without changing the parent.
`PeekBits` accepts 1 through 64 bits and preserves the reader's state on both
success and failure. Streams return
`ErrRandomAccessUnavailable` for those operations. Wrappers such as
`bufio.Reader` hide the underlying random-access interfaces and therefore
remain forward-only. `BitsRemaining` uses captured bounds for random-access
sources; for other seekable sources it performs the existing non-consuming
seeker query, and ordinary streams return `ErrRemainingBitsUnavailable`.
`CanFork` and `CanPeek` report this bounded random-access capability without
changing reader state; they have identical results and return false when
initialization failed.

#### Error-returning reads

```go
ReadBool() (bool, error)
ReadBits(bitCount uint8) (uint64, error)
PeekBits(bitCount uint8) (uint64, error)
ReadByte() (byte, error)
Read(data []byte) (int, error)
ReadBitsToSlice(bitCount uint64) ([]byte, error)
ReadBytesToSlice(byteCount uint64) ([]byte, error)
ReadStringToNull() (string, error)
ReadStringToLength(length uint64) (string, error)

ReadUint8() (uint8, error)
ReadInt8() (int8, error)

SkipBits(bitCount uint64) error
SkipBytes(byteCount uint64) error
Align() uint8
```

| Value | Configured byte order | Per-field byte order |
| --- | --- | --- |
| `uint16` | `ReadUint16() (uint16, error)` | `ReadUint16WithOrder(order ByteOrder) (uint16, error)` |
| `int16` | `ReadInt16() (int16, error)` | `ReadInt16WithOrder(order ByteOrder) (int16, error)` |
| `uint32` | `ReadUint32() (uint32, error)` | `ReadUint32WithOrder(order ByteOrder) (uint32, error)` |
| `int32` | `ReadInt32() (int32, error)` | `ReadInt32WithOrder(order ByteOrder) (int32, error)` |
| `uint64` | `ReadUint64() (uint64, error)` | `ReadUint64WithOrder(order ByteOrder) (uint64, error)` |
| `int64` | `ReadInt64() (int64, error)` | `ReadInt64WithOrder(order ByteOrder) (int64, error)` |
| `float32` | `ReadFloat32() (float32, error)` | `ReadFloat32WithOrder(order ByteOrder) (float32, error)` |
| `float64` | `ReadFloat64() (float64, error)` | `ReadFloat64WithOrder(order ByteOrder) (float64, error)` |

`ReadBits` accepts counts from 1 through 64 and returns its result in the low
bits of a `uint64`. It returns `io.EOF` when no requested bit is available and
`io.ErrUnexpectedEOF` when EOF follows a partial field; an incomplete field
returns zero and still advances the reader over its consumed bits. `Read`,
`ReadByte`, and scalar methods assemble logical bytes from the next eight
stream bits, so they also work at unaligned positions.
`ReadBitsToSlice` accepts any `uint64` bit count and returns `ceil(bitCount / 8)`
packed bytes: complete groups are logical bytes, and a final incomplete group
is stored in the low bits of the final byte using `ReadBits` bit-order
semantics. `ReadBytesToSlice` reads the requested number of logical bytes.
Both allocate their result, work at unaligned positions, and return only fully
read output with the read error if input ends early; an unfinished final bit
group is not included. A zero bit or byte count returns an empty slice without
consuming input. `ReadBitsToSlice` and `ReadBytesToSlice` allocate their requested result
before reading. `ReadStringToLength` allocates result capacity for the
requested length. Callers must validate input-derived lengths against an
application-specific allocation limit. The overflow checks only ensure that a
length fits in a Go slice, not that it is safe to allocate.
`ReadStringToNull` consumes but excludes its null terminator; if it is missing,
it returns the partial string and the read error. It has no maximum length, so
callers handling untrusted input should bound the source or otherwise enforce a
string limit. `ReadStringToLength` reads the specified number of logical bytes,
returns only the bytes before its first null terminator, and likewise returns a
partial string on a read error. It consumes the entire fixed-width field even
after finding a null terminator.

For a bounded child, `Align` stops at the next original byte boundary or that
child's end, whichever comes first.

#### `Must` reads

These methods panic with the original error returned by their error-returning
counterpart.

```go
MustReadBool() bool
MustReadBits(bitCount uint8) uint64
MustPeekBits(bitCount uint8) uint64
MustReadByte() byte
MustReadBitsToSlice(bitCount uint64) []byte
MustReadBytesToSlice(byteCount uint64) []byte
MustReadStringToNull() string
MustReadStringToLength(length uint64) string

MustReadUint8() uint8
MustReadInt8() int8
```

| Value | Configured byte order | Per-field byte order |
| --- | --- | --- |
| `uint16` | `MustReadUint16() uint16` | `MustReadUint16WithOrder(order ByteOrder) uint16` |
| `int16` | `MustReadInt16() int16` | `MustReadInt16WithOrder(order ByteOrder) int16` |
| `uint32` | `MustReadUint32() uint32` | `MustReadUint32WithOrder(order ByteOrder) uint32` |
| `int32` | `MustReadInt32() int32` | `MustReadInt32WithOrder(order ByteOrder) int32` |
| `uint64` | `MustReadUint64() uint64` | `MustReadUint64WithOrder(order ByteOrder) uint64` |
| `int64` | `MustReadInt64() int64` | `MustReadInt64WithOrder(order ByteOrder) int64` |
| `float32` | `MustReadFloat32() float32` | `MustReadFloat32WithOrder(order ByteOrder) float32` |
| `float64` | `MustReadFloat64() float64` | `MustReadFloat64WithOrder(order ByteOrder) float64` |

### Writer

`Writer` implements `io.Writer`, `io.ByteWriter`, and `io.Closer`.

```go
NewWriter(out io.Writer, options ...Option) *Writer

BitPosition() uint64
ByteAligned() bool
BitOrder() BitOrder
ByteOrder() ByteOrder
```

`BitPosition` includes padding added by `Align` and `Close`.

#### Error-returning writes

```go
WriteBool(value bool) error
WriteBits(value uint64, bitCount uint8) error
WriteByte(value byte) error
Write(data []byte) (int, error)
WriteString(value string) (int, error)

WriteUint8(value uint8) error
WriteInt8(value int8) error

PadToByte(value bool) (uint8, error)
Align() (uint8, error)
Close() error
```

| Value | Configured byte order | Per-field byte order |
| --- | --- | --- |
| `uint16` | `WriteUint16(value uint16) error` | `WriteUint16WithOrder(order ByteOrder, value uint16) error` |
| `int16` | `WriteInt16(value int16) error` | `WriteInt16WithOrder(order ByteOrder, value int16) error` |
| `uint32` | `WriteUint32(value uint32) error` | `WriteUint32WithOrder(order ByteOrder, value uint32) error` |
| `int32` | `WriteInt32(value int32) error` | `WriteInt32WithOrder(order ByteOrder, value int32) error` |
| `uint64` | `WriteUint64(value uint64) error` | `WriteUint64WithOrder(order ByteOrder, value uint64) error` |
| `int64` | `WriteInt64(value int64) error` | `WriteInt64WithOrder(order ByteOrder, value int64) error` |
| `float32` | `WriteFloat32(value float32) error` | `WriteFloat32WithOrder(order ByteOrder, value float32) error` |
| `float64` | `WriteFloat64(value float64) error` | `WriteFloat64WithOrder(order ByteOrder, value float64) error` |

`WriteBits` accepts counts from 1 through 64 and rejects a value that does not
fit in `bitCount` bits. `PadToByte` writes its value until the next byte boundary;
`Align` does the same with zero bits. `Close` zero-pads a partial byte, writes
it, and never closes the underlying `io.Writer`.
`WriteString` writes the raw bytes of its argument and implements
`io.StringWriter`.

#### `Must` writes

These methods panic with the original error returned by their error-returning
counterpart.

```go
MustWriteBool(value bool)
MustWriteBits(value uint64, bitCount uint8)
MustWriteByte(value byte)
MustWrite(data []byte) int
MustWriteString(value string) int

MustWriteUint8(value uint8)
MustWriteInt8(value int8)

MustPadToByte(value bool) uint8
MustAlign() uint8
MustClose()
```

| Value | Configured byte order | Per-field byte order |
| --- | --- | --- |
| `uint16` | `MustWriteUint16(value uint16)` | `MustWriteUint16WithOrder(order ByteOrder, value uint16)` |
| `int16` | `MustWriteInt16(value int16)` | `MustWriteInt16WithOrder(order ByteOrder, value int16)` |
| `uint32` | `MustWriteUint32(value uint32)` | `MustWriteUint32WithOrder(order ByteOrder, value uint32)` |
| `int32` | `MustWriteInt32(value int32)` | `MustWriteInt32WithOrder(order ByteOrder, value int32)` |
| `uint64` | `MustWriteUint64(value uint64)` | `MustWriteUint64WithOrder(order ByteOrder, value uint64)` |
| `int64` | `MustWriteInt64(value int64)` | `MustWriteInt64WithOrder(order ByteOrder, value int64)` |
| `float32` | `MustWriteFloat32(value float32)` | `MustWriteFloat32WithOrder(order ByteOrder, value float32)` |
| `float64` | `MustWriteFloat64(value float64)` | `MustWriteFloat64WithOrder(order ByteOrder, value float64)` |

### Errors

Operations can return underlying I/O errors and these exported sentinel errors:

| Error | Meaning |
| --- | --- |
| `ErrInvalidBitOrder` | A stream was configured with an unsupported `BitOrder`. |
| `ErrInvalidBitCount` | A bit operation requested fewer than 1 or more than 64 bits. |
| `ErrValueOverflow` | `WriteBits` received a value that does not fit in its requested width. |
| `ErrBitCountOverflow` | Converting a byte count to a bit count would overflow `uint64`. |
| `ErrStringLengthOverflow` | A requested string length cannot fit in a Go slice length. |
| `ErrSliceLengthOverflow` | A requested byte or packed-bit slice length cannot fit in a Go slice length. |
| `ErrRemainingBitsUnavailable` | The reader source cannot report its distance to EOF without consuming input. |
| `ErrRandomAccessUnavailable` | Forking or peeking was requested for a forward-only reader. |
| `ErrInvalidSize` | A random-access source reported a negative or inconsistent extent. |
| `ErrClosed` | A write was attempted after `Close`. |
| `ErrNilReader` | A reader has no usable source. |
| `ErrNilWriter` | A writer has no usable sink. |
| `ErrNilByteOrder` | A configured or per-field `ByteOrder` is nil. |

If a read fails partway through a bit field, the reader remains advanced by the
bits already consumed. A partial field ending at EOF returns
`io.ErrUnexpectedEOF`; an EOF before its first bit returns `io.EOF`. If an
underlying write fails, some bits may already have been emitted and the writer
cannot be used further.
