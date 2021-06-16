// package io providers utility functions for encoding and decoding
// low-level DICOM data types, such as integers and strings
package dicomio

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/sirupsen/logrus"
	"golang.org/x/text/encoding"
)

// ! ---- types/consts/variables ----

// NativeByteOrder is the byte order of this machine, auto-detect
var NativeByteOrder = binary.LittleEndian

type transferSyntaxStackEntry struct {
	byteorder binary.ByteOrder
	implicit  IsImplicitVR
}

type stackEntry struct {
	limit int64
	err   error
}

// Encoder is a helper class for encoding low-level DICOM data types
type Encoder struct {
	err error

	out io.Writer

	byteorder binary.ByteOrder

	// implicit不是内部方法 而是给user查看当前是implicit的transfer syntax
	implicit IsImplicitVR

	// Stack of old transfer syntaxes. {Push, Pop} TransferSyntax使用.
	oldTransferSyntaxes []transferSyntaxStackEntry
}

// NewBytesEncoder创建一个新的encoder，数据会写入缓冲区
// 可以通过Bytes（）来维持
func NewBytesEncoder(byteorder binary.ByteOrder, implicit IsImplicitVR) *Encoder {

	return &Encoder{
		err:       nil,
		out:       &bytes.Buffer{},
		byteorder: byteorder,
		implicit:  implicit,
	}
}

// 与NewBytesEncoder相似，但需要一个transfer syntax UID
func NewBytesEncoderWithTransferSyntax(transferSyntaxUID string) *Encoder {

	endian, implicit, err := ParseTransferSyntaxUID(transferSyntaxUID)

	if err == nil {
		return NewBytesEncoder(endian, implicit)
	}

	e := NewBytesEncoder(binary.LittleEndian, ExplicitVR)

	e.SetErrorf("%v: Unknown transfer syntax uid", transferSyntaxUID)

	return e
}

// 与NewEncoder相似, 但需要传一个transfer syntax UID.
func NewEncoderWithTransferSyntax(out io.Writer, transferSyntaxUID string) *Encoder {

	endian, implicit, err := ParseTransferSyntaxUID(transferSyntaxUID)
	if err == nil {
		return NewEncoder(out, endian, implicit)
	}

	e := NewEncoder(out, binary.LittleEndian, ExplicitVR)
	e.SetErrorf("%v: Unknown transfer syntax uid", transferSyntaxUID)

	return e
}

// NewEncoder creates a new encoder that writes to "out"
func NewEncoder(out io.Writer, byteorder binary.ByteOrder, implicit IsImplicitVR) *Encoder {

	return &Encoder{
		err:       nil,
		out:       out,
		byteorder: byteorder,
		implicit:  implicit,
	}
}

// TransferSyntax returns the current transfer syntax
func (e *Encoder) TransferSyntax() (binary.ByteOrder, IsImplicitVR) {
	return e.byteorder, e.implicit
}

// PushTransferSyntax() 暂时改变编码格式
// PopTransferSyntax() 来恢复
func (e *Encoder) PushTransferSyntax(byteorder binary.ByteOrder, implicit IsImplicitVR) {
	e.oldTransferSyntaxes = append(e.oldTransferSyntaxes,
		transferSyntaxStackEntry{e.byteorder, e.implicit})

	e.byteorder = byteorder
	e.implicit = implicit
}

// 与PushTransferSyntax对应
func (e *Encoder) PopTransferSyntax() {
	ts := e.oldTransferSyntaxes[len(e.oldTransferSyntaxes)-1]
	e.byteorder = ts.byteorder
	e.implicit = ts.implicit
	e.oldTransferSyntaxes = e.oldTransferSyntaxes[:len(e.oldTransferSyntaxes)-1]
}

// SetError sets the error to be reported by future Error() calls.
// If called multiple times with different errors, Error()
// will return one of the, but exactly which is unspecified.

// REQUIRES: err != nil
func (e *Encoder) SetError(err error) {
	if err != nil && e.err == nil {
		e.err = err
	}
}

// 与SetError相似，多传一个string
// SetErrorf is similar to SetError, but takes a printf format string
func (e *Encoder) SetErrorf(format string, args ...interface{}) {
	e.SetError(fmt.Errorf(format, args...))
}

// 返回一个由SetError设置的error，如果SetError没有被使用，则返回nil
func (e *Encoder) Error() error {
	return e.err
}

// Bytes returns the encoded data
//
// 须知: 由 Encoder 创建 NewBytesEncoder 而不是 NewEncoder
// 须知: e.Error() == nil
func (e *Encoder) Bytes() []byte {
	DoAssert(len(e.oldTransferSyntaxes) == 0)
	if e.err != nil {
		logrus.Panic(e.err)
	}
	return e.out.(*bytes.Buffer).Bytes()
}

func (e *Encoder) WriteByte(v byte) {
	// TODO warning？
	if err := binary.Write(e.out, e.byteorder, &v); err != nil {
		e.SetError(err)
	}
}

func (e *Encoder) WriteUInt16(v uint16) {
	if err := binary.Write(e.out, e.byteorder, &v); err != nil {
		e.SetError(err)
	}
}

func (e *Encoder) WriteUInt32(v uint32) {
	if err := binary.Write(e.out, e.byteorder, &v); err != nil {
		e.SetError(err)
	}
}

func (e *Encoder) WriteInt16(v int16) {
	if err := binary.Write(e.out, e.byteorder, &v); err != nil {
		e.SetError(err)
	}
}

func (e *Encoder) WriteInt32(v int32) {
	if err := binary.Write(e.out, e.byteorder, &v); err != nil {
		e.SetError(err)
	}
}

func (e *Encoder) WriteFloat32(v float32) {
	if err := binary.Write(e.out, e.byteorder, &v); err != nil {
		e.SetError(err)
	}
}

func (e *Encoder) WriteFloat64(v float64) {
	if err := binary.Write(e.out, e.byteorder, &v); err != nil {
		e.SetError(err)
	}
}

// WriteString writes the string, withoutout any length prefix or padding.
func (e *Encoder) WriteString(v string) {
	if _, err := e.out.Write([]byte(v)); err != nil {
		e.SetError(err)
	}
}

// WriteZeros encodes an array of zero bytes.
func (e *Encoder) WriteZeros(len int) {
	// TODO 重用缓存
	zeros := make([]byte, len)
	e.out.Write(zeros)
}

// Copy the given data to output.
func (e *Encoder) WriteBytes(v []byte) {
	e.out.Write(v)
}

// IsImplicitVR defines whether a 2-character VR tag
// is emit with each data element
// IsImplicitVR定义了2字节的VR tag是否是与他的data element 对应（emit）？
type IsImplicitVR int

const (

	// TODO implicit/explicit在哪里被定义？加一个reference

	// ImplicitVR编码一个没有VR tag的data element
	// 从dicom standard静态页面(tags.go) 来读取 tag->VR的对应
	ImplicitVR IsImplicitVR = iota

	// ExplicitVR 保存了2比特VR value inline w/ a data element
	ExplicitVR

	// UnknownVR is to be used when you never encode or decode DataElement.
	// UnknownVR用来定义一个没有编码或解码的DataElement？
	UnknownVR
)

// Decoder用来解码low-level的dicom data 类型（types）
type Decoder struct {
	in        *bufio.Reader
	err       error
	byteorder binary.ByteOrder

	// “implicit”不是由docoder内部使用，是让docoder的使用者可以看见当前的transfer syntax
	implicit IsImplicitVR

	// 可以读进的最大比特数
	limit int64

	// Cumulative # bytes read.
	pos int64

	// 将dicom文件的原始数据解码为utf-8，如果为空，则可能是ASCII编码。详情见Cf p3.5 6.1.2.1
	codingSystem CodingSystem

	// 旧transfer syntax栈，由{push, pop}TransferSyntax使用
	oldTransferSyntaxes []transferSyntaxStackEntry
	// 旧limit栈，由{push, pop}Limit使用
	// oldLimits[] 以降序存储
	stateStack []stackEntry
}

// NewDecoder创建一个decoder对象从"in"读取“limit”
// 不要随便传一个大数来作为"limit"，如下代码会认为"limit"绑定在data最后
func NewDecoder(
	in io.Reader,
	byteorder binary.ByteOrder,
	implicit IsImplicitVR) *Decoder {
	return &Decoder{
		in:        bufio.NewReader(in),
		err:       nil,
		byteorder: byteorder,
		implicit:  implicit,
		pos:       0,
		limit:     math.MaxInt64,
	}
}

// NewBytesDecoder 创建一个decoder来读取“a sequence of bytes”。
// 详情对比NewDecoder
func NewBytesDecoder(data []byte, byteorder binary.ByteOrder, implicit IsImplicitVR) *Decoder {
	return NewDecoder(bytes.NewReader(data), byteorder, implicit)
}

// NewBytesDecoderWithTransferSyntax与NewBytesDecoder相似，
// 但需要一个transfer syntax UID 而不是一对<byteorder, IsImplicitVR>
func NewBytesDecoderWithTransferSyntax(data []byte, transferSyntaxUID string) *Decoder {

	endian, implicit, err := ParseTransferSyntaxUID(transferSyntaxUID)
	if err == nil {
		return NewBytesDecoder(data, endian, implicit)
	}

	d := NewBytesDecoder(data, binary.LittleEndian, ExplicitVR)
	d.SetError(fmt.Errorf("%v: Unknown transfer syntax uid", transferSyntaxUID))
	return d
}

// SetError 将之后Error() 或 Finish() call的错误设为已上报（reported）
// 要求: err != nil
func (d *Decoder) SetError(err error) {
	if err != nil && d.err == nil {
		if err != io.EOF {
			err = fmt.Errorf("%s (file offset %d)", err.Error(), d.pos)
		}
		d.err = err
	}
}

// SetErrorf 与 SetError相似，但需要一个可打印的string
func (d *Decoder) SetErrorf(format string, args ...interface{}) {
	d.SetError(fmt.Errorf(format, args...))
}

// TransferSyntax 返回目前的transfer syntax
func (d *Decoder) TransferSyntax() (byteorder binary.ByteOrder, implicit IsImplicitVR) {

	return d.byteorder, d.implicit
}

// PushTransferSyntax() 暂时改变编码格式
// PopTransferSyntax() 恢复旧的编码格式
func (d *Decoder) PushTransferSyntax(byteorder binary.ByteOrder, implicit IsImplicitVR) {

	d.oldTransferSyntaxes = append(d.oldTransferSyntaxes, transferSyntaxStackEntry{d.byteorder, d.implicit})
	d.byteorder = byteorder
	d.implicit = implicit
}

// PushTransferSyntaxByUID is similar to PushTransferSyntax, but it takes a
// transfer syntax UID.
func (d *Decoder) PushTransferSyntaxByUID(uid string) {
	endian, implicit, err := ParseTransferSyntaxUID(uid)
	if err != nil {
		d.SetError(err)
	}
	d.PushTransferSyntax(endian, implicit)
}

// SetCodingSystem overrides the default (7bit ASCII) decoder used when
// converting a byte[] to a string.
func (d *Decoder) SetCodingSystem(cs CodingSystem) {
	d.codingSystem = cs
}

// PopTransferSyntax 在最后一次调用PushTransferSyntax前回复编码方式
func (d *Decoder) PopTransferSyntax() {

	e := d.oldTransferSyntaxes[len(d.oldTransferSyntaxes)-1]

	d.byteorder = e.byteorder
	d.implicit = e.implicit
	d.oldTransferSyntaxes = d.oldTransferSyntaxes[:len(d.oldTransferSyntaxes)-1]
}

// PushLimit 暂时重写缓冲尾(end of buffer)和清除d.err
// PopLimit 会恢复旧的limit和error
//
// 注意：新的limit必须比当前的limit小
func (d *Decoder) PushLimit(bytes int64) {
	newLimit := d.pos + bytes
	if newLimit > d.limit {
		d.SetError(fmt.Errorf("trying to read %d bytes beyond buffer end", newLimit-d.limit))
		newLimit = d.pos
	}
	d.stateStack = append(d.stateStack, stackEntry{limit: d.limit, err: d.err})
	d.limit = newLimit
	d.err = nil
}

// PopLimit 恢复由PushLimit覆盖的limit
func (d *Decoder) PopLimit() {
	if d.pos < d.limit {
		// d.pos < d.limit iff parse error happened and the caller didn't fully
		// consume the input.  Here we skip over the unparsable part.  This is just a
		// heuristics to parse as much data as possible from corrupt files.
		d.Skip(int(d.limit - d.pos))
	}
	last := len(d.stateStack) - 1
	d.limit = d.stateStack[last].limit
	if d.stateStack[last].err != nil {
		d.err = d.stateStack[last].err
	}
	d.stateStack = d.stateStack[:last]
}

// Error returns an error encountered so far.
func (d *Decoder) Error() error { return d.err }

// finish()必须在使用decoder之后用
// 会返回在运行decoder中遇到的任何错误
// 如果有data无法被处理 也会返回一个错误
func (d *Decoder) Finish() error {
	if d.err != nil {
		return d.err
	}
	if !d.EOF() {
		return fmt.Errorf("Decoder found junk")
	}
	return nil
}

func (d *Decoder) Read(p []byte) (int, error) {

	desired := d.len()
	if desired == 0 {
		if len(p) == 0 {
			return 0, nil
		}

		return 0, io.EOF
	}

	if desired < int64(len(p)) {
		p = p[:desired]
	}

	n, err := d.in.Read(p)
	if n >= 0 {
		d.pos += int64(n)
	}

	return n, err
}

// EOF 检查如果没有可读数据了
func (d *Decoder) EOF() bool {
	if d.err != nil {
		return true
	}

	if d.limit-d.pos <= 0 {
		return true
	}

	data, _ := d.in.Peek(1)
	return len(data) == 0
}

// BytesRead returns the cumulative # of bytes read so far.
func (d *Decoder) BytesRead() int64 { return d.pos }

// Len 返回 当前读取的bytes数
func (d *Decoder) len() int64 {

	return d.limit - d.pos
}

// ReadByte reads a single byte from the buffer. On EOF, it returns a junk
// value, and sets an error to be returned by Error() or Finish().
func (d *Decoder) ReadByte() (v byte) {
	err := binary.Read(d, d.byteorder, &v)
	if err != nil {
		d.SetError(err)
		return 0
	}
	return v
}

func (d *Decoder) ReadUInt32() (v uint32) {
	err := binary.Read(d, d.byteorder, &v)
	if err != nil {
		d.SetError(err)
	}
	return v
}

func (d *Decoder) ReadInt32() (v int32) {
	err := binary.Read(d, d.byteorder, &v)
	if err != nil {
		d.SetError(err)
	}
	return v
}

func (d *Decoder) ReadUInt16() (v uint16) {
	err := binary.Read(d, d.byteorder, &v)
	if err != nil {
		d.SetError(err)
	}
	return v
}

func (d *Decoder) ReadInt16() (v int16) {
	err := binary.Read(d, d.byteorder, &v)
	if err != nil {
		d.SetError(err)
	}
	return v
}

func (d *Decoder) ReadFloat32() (v float32) {
	err := binary.Read(d, d.byteorder, &v)
	if err != nil {
		d.SetError(err)
	}
	return v
}

func (d *Decoder) ReadFloat64() (v float64) {
	err := binary.Read(d, d.byteorder, &v)
	if err != nil {
		d.SetError(err)
	}
	return v
}

func internalReadString(d *Decoder, sd *encoding.Decoder, length int) string {

	bytes := d.ReadBytes(length)
	if len(bytes) == 0 {
		return ""
	}

	if sd == nil {
		// 假设UTF-8是ASCII的超集
		// TODO check that string is 7-bit clean？
		return string(bytes)
	}

	bytes, err := sd.Bytes(bytes)
	if err != nil {
		d.SetError(err)
		return ""
	}

	return string(bytes)
}

func (d *Decoder) ReadStringWithCodingSystem(csType CodingSystemType, length int) string {
	var sd *encoding.Decoder
	switch csType {
	case AlphabeticCodingSystem:
		sd = d.codingSystem.Alphabetic
	case IdeographicCodingSystem:
		sd = d.codingSystem.Ideographic
	case PhoneticCodingSystem:
		sd = d.codingSystem.Phonetic
	default:
		panic(csType)
	}
	return internalReadString(d, sd, length)
}

func (d *Decoder) ReadString(length int) string {

	return internalReadString(d, d.codingSystem.Ideographic, length)
}

func (d *Decoder) ReadBytes(length int) []byte {
	if d.len() < int64(length) {
		d.SetError(fmt.Errorf("ReadBytes: requested %d, available %d", length, d.len()))
		return nil
	}
	v := make([]byte, length)
	remaining := v
	for len(remaining) > 0 {
		n, err := d.Read(remaining)
		if err != nil {
			d.SetError(err)
			break
		}
		if n < 0 || n > len(remaining) {
			panic(fmt.Sprintf("Remaining: %d %d", n, len(remaining)))
		}
		remaining = remaining[n:]
	}
	DoAssert(d.err != nil || len(remaining) == 0)
	return v
}

func (d *Decoder) Skip(length int) {

	if d.len() < int64(length) {
		d.SetError(fmt.Errorf("Skip: requested %d, available %d",
			length, d.len()))
		return
	}

	// 位运算
	junkSize := 1 << 16
	if length < junkSize {
		junkSize = length
	}

	junk := make([]byte, junkSize)

	remaining := length
	for remaining > 0 {
		tempLength := len(junk)
		if remaining < tempLength {
			tempLength = remaining
		}

		tempBuf := junk[:tempLength]
		n, err := d.Read(tempBuf)
		if err != nil {
			d.SetError(err)
			break
		}
		DoAssert(n > 0)
		remaining -= n
	}

	DoAssert(d.err != nil || remaining == 0)
}

func DoAssert(condition bool, values ...interface{}) {

	if !condition {
		var s string
		for _, value := range values {
			s += fmt.Sprintf("%v", value)
		}

		logrus.Panic(s)
	}
}
