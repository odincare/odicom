package dicom

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/odincare/odicom/dicomio"
	"github.com/odincare/odicom/dicomtag"

	"github.com/sirupsen/logrus"
)

// Element represents a single DICOM element. Use NewElement() to create a
// element denovo. Avoid creating a struct manually, because setting the VR
// field is a bit tricky.
type Element struct {
	// Tag is a pair of <group, element>. See tags.go for possible values.
	Tag dicomtag.Tag

	// List of values in the element. Their types depends on value
	// representation (VR) of the Tag; Cf. tag.go.
	//
	// If Tag==TagPixelData, len(Value)==1, and Value[0] is PixelDataInfo.
	// Else if Tag==TagItem, each Value[i] is a *Element.
	//    a value's Tag can be any (including TagItem, which represents a nested Item)
	// Else if VR=="SQ", Value[i] is a *Element, with Tag=TagItem.
	// Else if VR=="LT", or "UT", then len(Value)==1, and Value[0] is string
	// Else if VR=="DA", then len(Value)==1, and Value[0] is string. Use ParseDate() to parse the date string.
	// Else if VR=="US", Value[] is a list of uint16s (len(Value) matches VM of the Tag; PS 3.5 6.4)
	// Else if VR=="UL", Value[] is a list of uint32s (len(Value) matches VM of the Tag; PS 3.5 6.4)
	// Else if VR=="SS", Value[] is a list of int16s (len(Value) matches VM of the Tag; PS 3.5 6.4)
	// Else if VR=="SL", Value[] is a list of int32s (len(Value) matches VM of the Tag; PS 3.5 6.4)
	// Else if VR=="FL", Value[] is a list of float32s (len(Value) matches VM of the Tag; PS 3.5 6.4)
	// Else if VR=="FD", Value[] is a list of float64s (len(Value) matches VM of the Tag; PS 3.5 6.4)
	// Else if VR=="AT", Value[] is a list of Tag's. (len(Value) matches VM of the Tag; PS 3.5 6.4)
	// Else if VR=="OF", Value[] is a list of float32s
	// Else if VR=="OD", Value[] is a list of float64s
	// Else if VR=="OW" or "OB", len(Value)==1, and Value[0] is []byte.
	// Else, Value[] is a list of strings.
	//
	// Note: Use GetVRKind() to map VR string to the go representation of
	// VR.
	Value []interface{} // Value Multiplicity PS 3.5 6.4

	// Note: the following fields are not interesting to most people, but
	// are filled for completeness.  You can ignore them.

	// VR defines the encoding of Value[] in two-letter alphabets, e.g.,
	// "AE", "UL". See P3.5 6.2. This field MUST be set.
	//
	// dicom.ReadElement() will fill this field with the VR of the tag,
	// either read from input stream (for explicit repl), or from the dicom
	// tag table (for implicit decl). This field need not be set in
	// WriteElement().
	//
	// Note: In a conformant DICOM file, the VR value of an element is
	// determined by its Tag, so this field is redundant.  This field is
	// still required because a non-conformant file with with explicitVR
	// encoding may have an element with VR that's different from the
	// standard's. In such case, this library honors the VR value found in
	// the file, and this field stores the VR used for parsing Values[].
	VR string

	// UndefinedLength is true if, in the DICOM file, the element is encoded
	// as having undefined length, and is delimited by end-sequence or
	// end-item element.  This flag is meaningful only if VR=="SQ" or
	// VR=="NA". Feel free to ignore this field if you don't understand what
	// this means.  It's one of the pointless complexities in the DICOM
	// standard.
	UndefinedLength bool
}

type DataSet struct {
	// 与pydicom不同， Elements扔包含元数据（Tag.Group==2的)
	Elements []*Element
}

// ReadOptions定义DataSets和Element的读取格式
type ReadOptions struct {
	// DropPixelData会让ReadDataSet跳过PixelData(bulk image)
	DropPixelData bool

	// ReturnTags 会返回一系列tag白名单
	ReturnTags []dicomtag.Tag

	//TODO (翻译有点问题) StopAtTag 使在读取时或value超过最大值时，程序会停止读取dicom file
	StopAtTag *dicomtag.Tag
}

type PixelDataInfo struct {
	Offsets []uint32 // BasicOffsetTable
	Frames  [][]byte // Parsed images
}

const UndefinedLength uint32 = 0xffffffff

const ItemSeqGroup = 0xFFFE

// NewElement用传入的tag和values来创建一个新的Element
// 每个传入的值必须符合 tag 的 VR
// 详情-> tag_definition.go
func NewElement(tag dicomtag.Tag, values ...interface{}) (*Element, error) {
	ti, err := dicomtag.Find(tag)
	if err != nil {
		return nil, err
	}

	e := Element{
		Tag:   tag,
		VR:    ti.VR,
		Value: make([]interface{}, len(values)),
	}

	vrKind := dicomtag.GetVRKind(tag, ti.VR)

	for i, v := range values {
		var ok bool

		switch vrKind {
		case dicomtag.VRStringList, dicomtag.VRDate:
			_, ok = v.(string)
		case dicomtag.VRBytes:
			_, ok = v.([]byte)
		case dicomtag.VRUInt16List:
			_, ok = v.(uint16)
		case dicomtag.VRUInt32List:
			_, ok = v.(uint32)
		case dicomtag.VRInt16List:
			_, ok = v.(int16)
		case dicomtag.VRInt32List:
			_, ok = v.(int32)
		case dicomtag.VRFloat32List:
			_, ok = v.(float32)
		case dicomtag.VRFloat64List:
			_, ok = v.(float64)
		case dicomtag.VRPixelData:
			_, ok = v.(PixelDataInfo)
		case dicomtag.VRTagList:
			_, ok = v.(dicomtag.Tag)
		case dicomtag.VRSequence:
			var subelement *Element
			subelement, ok = v.(*Element)
			if ok {
				ok = (subelement.Tag == dicomtag.Item)
			}
		case dicomtag.VRItem:
			_, ok = v.(*Element)
		}

		if !ok {
			return nil, fmt.Errorf("%v: wrong payload type for NewElement: expect %v, but found %v",
				dicomtag.DebugString(tag), vrKind, v)
		}

		e.Value[i] = v
	}

	return &e, nil
}

// MustNewElement is similar to NewElement, but it crashes the process on any error
func MustNewElement(tag dicomtag.Tag, values ...interface{}) *Element {

	elem, err := NewElement(tag, values...)
	if err != nil {
		panic(fmt.Sprintf("Failed to create element with tag %v: %v", tag, err))
	}

	return elem
}

// GetUInt32 gets a uint32 value from an element.  It returns an error if the
// element contains zero or >1 values, or the value is not a uint32.
func (e *Element) GetUInt32() (uint32, error) {
	if len(e.Value) != 1 {
		return 0, fmt.Errorf("Found %d value(s) in getuint32 (expect 1): %v", len(e.Value), e)
	}
	v, ok := e.Value[0].(uint32)
	if !ok {
		return 0, fmt.Errorf("Uint32 value not found in %v", e)
	}
	return v, nil
}

// MustGetUInt32 is similar to GetUInt32, but panics on error.
func (e *Element) MustGetUInt32() uint32 {
	v, err := e.GetUInt32()
	if err != nil {
		panic(err)
	}
	return v
}

// GetUInt16 gets a uint16 value from an element.  It returns an error if the
// element contains zero or >1 values, or the value is not a uint16.
func (e *Element) GetUInt16() (uint16, error) {
	if len(e.Value) != 1 {
		return 0, fmt.Errorf("Found %d value(s) in getuint16 (expect 1): %v", len(e.Value), e)
	}
	v, ok := e.Value[0].(uint16)
	if !ok {
		return 0, fmt.Errorf("Uint16 value not found in %v", e)
	}
	return v, nil
}

// MustGetUInt16 is similar to GetUInt16, but panics on error.
func (e *Element) MustGetUInt16() uint16 {
	v, err := e.GetUInt16()
	if err != nil {
		panic(err)
	}
	return v
}

// GetString gets a string value from an element.  It returns an error if the
// element contains zero or >1 values, or the value is not a string.
func (e *Element) GetString() (string, error) {
	if len(e.Value) != 1 {
		return "", fmt.Errorf("Found %d value(s) in getstring (expect 1): %v", len(e.Value), e.String())
	}
	v, ok := e.Value[0].(string)
	if !ok {
		return "", fmt.Errorf("string value not found in %v", e)
	}
	return v, nil
}

// MustGetString is similar to GetString(), but panics on error.
//
// TODO Add other variants of MustGet<type>.
func (e *Element) MustGetString() string {
	v, err := e.GetString()
	if err != nil {
		panic(err)
	}
	return v
}

// GetStrings 返回 存在element中的string数组，
// 如果 e.Tag的VR不是string将返回错误
func (e *Element) GetStrings() ([]string, error) {
	values := make([]string, 0, len(e.Value))
	for _, v := range e.Value {
		v, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("string value not found in %v", e.String())
		}

		values = append(values, v)
	}

	return values, nil
}

// GetUint32s returns the list of uint32 values stored in the elment. Returns an
// error if the VR of e.Tag is not a uint32.
func (e *Element) GetUint32s() ([]uint32, error) {
	values := make([]uint32, 0, len(e.Value))
	for _, v := range e.Value {
		v, ok := v.(uint32)
		if !ok {
			return nil, fmt.Errorf("uint32 value not found in %v", e.String())
		}
		values = append(values, v)
	}
	return values, nil
}

// MustGetUint32s is similar to GetUint32s, but crashes the process on error.
func (e *Element) MustGetUint32s() []uint32 {
	values, err := e.GetUint32s()
	if err != nil {
		panic(err)
	}
	return values
}

// GetUint16s returns the list of uint16 values stored in the elment. Returns an
// error if the VR of e.Tag is not a uint16.
func (e *Element) GetUint16s() ([]uint16, error) {
	values := make([]uint16, 0, len(e.Value))
	for _, v := range e.Value {
		v, ok := v.(uint16)
		if !ok {
			return nil, fmt.Errorf("uint16 value not found in %v", e.String())
		}
		values = append(values, v)
	}
	return values, nil
}

// MustGetUint16s is similar to GetUint16s, but crashes the process on error.
func (e *Element) MustGetUint16s() []uint16 {
	values, err := e.GetUint16s()
	if err != nil {
		panic(err)
	}
	return values
}

func elementString(e *Element, nestLevel int) string {
	dicomio.DoAssert(nestLevel < 10)
	indent := strings.Repeat(" ", nestLevel)
	s := indent
	sVl := ""
	if e.UndefinedLength {
		sVl = "u"
	}
	s = fmt.Sprintf("%s %s %s %s ", s, dicomtag.DebugString(e.Tag), e.VR, sVl)
	if e.VR == "SQ" || e.Tag == dicomtag.Item {
		s += fmt.Sprintf(" (#%d)[\n", len(e.Value))
		for _, v := range e.Value {
			s += elementString(v.(*Element), nestLevel+1) + "\n"
		}
		s += indent + " ]"
	} else {
		var sv string
		if len(e.Value) == 1 {
			sv = fmt.Sprintf("%v", e.Value)
		} else {
			sv = fmt.Sprintf("(%d)%v", len(e.Value), e.Value)
		}
		if len(sv) > 1024 {
			sv = sv[1:1024] + "(...)"
		}
		s += sv
	}
	return s
}

// Stringer
func (e *Element) String() string {
	return elementString(e, 0)
}

// 读取一个Item object的元数据，w/o 读取它们进DataElement.
// 它是用来读取 pixel data的
func readRawItem(d *dicomio.Decoder) ([]byte, bool) {

	tag := readTag(d)

	// Item总是显示的, PS3.6 7.5
	vr, vl := readImplicit(d, tag)

	if d.Error() != nil {
		return nil, true
	}

	if tag == dicomtag.SequenceDelimitationItem {
		if vl != 0 {
			d.SetErrorf("SequenceDelimitationItem's VL != 0: %v", vl)
		}
		return nil, true
	}

	if tag != dicomtag.Item {
		d.SetErrorf("Expect Item in pixelData but fount tag %v", dicomtag.DebugString(tag))
		return nil, false
	}

	if vl == UndefinedLength {
		d.SetErrorf("Expect defined-length item in pixelData")
		return nil, false
	}

	if vr != "NA" {
		d.SetErrorf("Expect NA item, but fount %s", vr)
		return nil, true
	}

	return d.ReadBytes(int(vl)), false
}

// 读取 basic offset table。 这是PixelData内的第一个 embedded 对象
// P3.5 8.2 P3.5 A4 有更好的示例
func readBasicOffsetTable(d *dicomio.Decoder) []uint32 {

	data, endOfData := readRawItem(d)
	if endOfData {
		d.SetErrorf("basic offset table not found")
	}

	if len(data) == 0 {
		return []uint32{0}
	}

	byteOrder, _ := d.TransferSyntax()

	// item的值是uint32的序列，每个值代表接下来图片的大小（byte size）
	subdecoder := dicomio.NewBytesDecoder(data, byteOrder, dicomio.ImplicitVR)

	var offsets []uint32
	for !subdecoder.EOF() {
		offsets = append(offsets, subdecoder.ReadUInt32())
	}

	return offsets
}

// ParseFileHeader从Dicom文件读取DICOM头和元数据(element的tag group == 2的)
// 报错会通过d.Error()传入
func ParseFileHeader(d *dicomio.Decoder) []*Element {

	d.PushTransferSyntax(binary.LittleEndian, dicomio.ExplicitVR)
	defer d.PopTransferSyntax()

	// 跳过前言
	d.Skip(128)

	// Check for magic word
	if s := d.ReadString(4); s != "DICM" {
		// bom头没找到DICM
		d.SetError(errors.New("keyword 'DICM' not found in the header"))
		return nil
	}

	// (0002, 0000) MetaElementGroupLength
	metaElement := ReadElement(d, ReadOptions{})

	if d.Error() != nil {
		return nil
	}
	if metaElement.Tag != dicomtag.FileMetaInformationGroupLength {
		d.SetErrorf("MetaElementGroupLength not found; insteadfound %s", metaElement.Tag.String())
	}
	metaLength, err := metaElement.GetUInt32()
	if err != nil {
		d.SetErrorf("Failed to read uint32 in MetaElementGroupLength: %v", err)
		return nil
	}
	if d.EOF() {
		d.SetErrorf("No data element found")
		return nil
	}
	metaElems := []*Element{metaElement}

	// Read meta tags
	d.PushLimit(int64(metaLength))
	defer d.PopLimit()
	for !d.EOF() {
		elem := ReadElement(d, ReadOptions{})
		if d.Error() != nil {
			break
		}
		metaElems = append(metaElems, elem)
		logrus.Infof("dicom.ParseFileHeader: Meta element: %v, pos %v", elem.String(), d.BytesRead())
	}
	return metaElems
}

// endElement 是一个伪元素来导致caller停止读取input
var endOfDataElement = &Element{Tag: dicomtag.Tag{Group: 0x7fff, Element: 0x7fff}}

// ReadElement 读取一个DICOM data element，返回三种值.
//
// - 读取错误时，返回nil和d.Error()错误的集合
//
// - 返回(endOfDataElement, nil) 如果options.DropPixelData为true且
// element 是 pixel data， 或者遇到一个option.StopAtTag
//
// - 读取成功时，返回一个non-nil 和 non-endOfDataElement 值
func ReadElement(d *dicomio.Decoder, options ReadOptions) *Element {

	tag := readTag(d)
	if tag == dicomtag.PixelData && options.DropPixelData {
		return endOfDataElement
	}

	// 如果有StopAtTag且tag比StopAtTag大
	if options.StopAtTag != nil && tag.Group >= options.StopAtTag.Group && tag.Element >= options.StopAtTag.Element {
		return endOfDataElement
	}

	// 组为0xFFFE 的 elements组应被编码为Implicit VR
	// DICOM 标准09. PS3.6 - Section 7.5: "Nesting of Data Sets"
	_, implicit := d.TransferSyntax()
	if tag.Group == ItemSeqGroup {
		implicit = dicomio.ImplicitVR
	}

	var vr string
	var vl uint32

	if implicit == dicomio.ImplicitVR {
		vr, vl = readImplicit(d, tag)
	} else {
		dicomio.DoAssert(implicit == dicomio.ExplicitVR, implicit)

		vr, vl = readExplicit(d, tag)
	}

	var data []interface{}

	elem := &Element{
		Tag:             tag,
		VR:              vr,
		UndefinedLength: (vl == UndefinedLength),
	}

	if vr == "UN" && vl == UndefinedLength {
		// 有些文件会有这种组合，不明白是干什么，标准也没有说清楚
		// 在PS3.5, 6.2.2中说也许会出现vr=UN且未定义的长度是被允许的
		// 它被引用于：读取一个"Data Elements with Unknown Length"
		// 那个引用是专用于type=SQ的，所以他猜测
		// <UN, undefinedLength> == <SQ, undefinedLength>
		vr = "SQ"
		elem.VR = vr
	}

	if tag == dicomtag.PixelData {
		// P3.5, A.4 describes the format. Currently we only support an encapsulated image format.
		//
		// PixelData is usually the last element in a DICOM file. When
		// the file stores N images, the elements that follow PixelData
		// are laid out in the following way:
		//
		// Item(BasicOffsetTable) Item(PixelDataInfo0) ... Item(PixelDataInfoM) SequenceDelimiterItem
		//
		// Item(BasicOffsetTable) is an Item element whose payload
		// encodes N uint32 values. Kth uint32 is the bytesize of the
		// Kth image. Item(PixelDataInfo*) are chunked sequences of bytes. I
		// presume that single PixelDataInfo item doesn't cross a image
		// boundary, but the spec isn't clear.
		//
		// The total byte size of Item(PixelDataInfo*) equal the total of
		// the bytesizes found in BasicOffsetTable.

		if vl == UndefinedLength {
			var image PixelDataInfo
			image.Offsets = readBasicOffsetTable(d)

			if len(image.Offsets) > 1 {
				logrus.Warnf("ReadElement: Multiple images not supported yet, Combining them into a byte sequence: %v", image.Offsets)
			}

			for !d.EOF() {
				chunk, endOfItems := readRawItem(d)
				if d.Error() != nil {
					break
				}

				if endOfItems {
					break
				}

				image.Frames = append(image.Frames, chunk)
			}

			data = append(data, image)
		} else {
			logrus.Warnf("ReadElement: Defined-length pixel data not supported: tag %v, VR=%v, VL=%v", tag.String(), vr, vl)

			var image PixelDataInfo

			image.Frames = append(image.Frames, d.ReadBytes(int(vl)))
			data = append(data, image)
		}
		// TODO 处理多帧图片
	} else if vr == "SQ" {
		// Note: when reading subitems inside sequence or item, we ignore
		// DropPixelData and other shortcircuiting options. If we honored them, we'd
		// be unable to read the rest of the file.
		if vl == UndefinedLength {
			// Format:
			//  Sequence := ItemSet* SequenceDelimitationItem
			//  ItemSet := Item Any* ItemDelimitationItem (when Item.VL is undefined) or
			//             Item Any*N                     (when Item.VL has a defined value)
			for {
				// Makes sure to return all sub elements even if the tag is not in the return tags list of options or is greater than the Stop At Tag
				item := ReadElement(d, ReadOptions{})
				if d.Error() != nil {
					break
				}
				if item.Tag == dicomtag.SequenceDelimitationItem {
					break
				}
				if item.Tag != dicomtag.Item {
					d.SetErrorf("dicom.ReadElement: Found non-Item element in seq w/ undefined length: %v", dicomtag.DebugString(item.Tag))
					break
				}
				data = append(data, item)
			}
		} else {
			// Format:
			//  Sequence := ItemSet*VL
			// See the above comment for the definition of ItemSet.
			d.PushLimit(int64(vl))
			for !d.EOF() {
				// Makes sure to return all sub elements even if the tag is not in the return tags list of options or is greater than the Stop At Tag
				item := ReadElement(d, ReadOptions{})
				if d.Error() != nil {
					break
				}
				if item.Tag != dicomtag.Item {
					d.SetErrorf("dicom.ReadElement: Found non-Item element in seq w/ undefined length: %v", dicomtag.DebugString(item.Tag))
					break
				}
				data = append(data, item)
			}
			d.PopLimit()
		}
	} else if tag == dicomtag.Item { // Item (component of SQ)
		if vl == UndefinedLength {
			// Format: Item Any* ItemDelimitationItem
			for {
				// Makes sure to return all sub elements even if the tag is not in the return tags list of options or is greater than the Stop At Tag
				subelem := ReadElement(d, ReadOptions{})
				if d.Error() != nil {
					break
				}
				if subelem.Tag == dicomtag.ItemDelimitationItem {
					break
				}
				data = append(data, subelem)
			}
		} else {
			// Sequence of arbitrary elements, for the  total of "vl" bytes.
			d.PushLimit(int64(vl))
			for !d.EOF() {
				// Makes sure to return all sub elements even if the tag is not in the return tags list of options or is greater than the Stop At Tag
				subelem := ReadElement(d, ReadOptions{})
				if d.Error() != nil {
					break
				}
				data = append(data, subelem)
			}
			d.PopLimit()
		}
	} else { // List of scalar
		if vl == UndefinedLength {
			d.SetErrorf("dicom.ReadElement: Undefined length disallowed for VR=%s, tag %s", vr, dicomtag.DebugString(tag))
			return nil
		}
		d.PushLimit(int64(vl))
		defer d.PopLimit()
		if vr == "DA" {
			// TODO(saito) Maybe we should validate the date.
			date := strings.Trim(d.ReadString(int(vl)), " \000")
			data = []interface{}{date}
		} else if vr == "AT" {
			// (2byte group, 2byte elem)
			for !d.EOF() {
				tag := dicomtag.Tag{d.ReadUInt16(), d.ReadUInt16()}
				data = append(data, tag)
			}
		} else if vr == "OW" {
			if vl%2 != 0 {
				d.SetErrorf("dicom.ReadElement: tag %v: OW requires even length, but found %v", dicomtag.DebugString(tag), vl)
			} else {
				n := int(vl / 2)
				e := dicomio.NewBytesEncoder(dicomio.NativeByteOrder, dicomio.UnknownVR)
				for i := 0; i < n; i++ {
					v := d.ReadUInt16()
					e.WriteUInt16(v)
				}
				dicomio.DoAssert(e.Error() == nil, e.Error())
				// TODO Check that size is even. Byte swap??
				// TODO If OB's length is odd, is VL odd too? Need to check!
				data = append(data, e.Bytes())
			}
		} else if vr == "OB" {
			// TODO Check that size is even. Byte swap??
			// TODO If OB's length is odd, is VL odd too? Need to check!
			data = append(data, d.ReadBytes(int(vl)))
		} else if vr == "LT" || vr == "UT" {
			str := d.ReadString(int(vl))
			data = append(data, str)
		} else if vr == "UL" {
			for !d.EOF() {
				data = append(data, d.ReadUInt32())
			}
		} else if vr == "SL" {
			for !d.EOF() {
				data = append(data, d.ReadInt32())
			}
		} else if vr == "US" {
			for !d.EOF() {
				data = append(data, d.ReadUInt16())
			}
		} else if vr == "SS" {
			for !d.EOF() {
				data = append(data, d.ReadInt16())
			}
		} else if vr == "FL" || vr == "OF" {
			for !d.EOF() {
				data = append(data, d.ReadFloat32())
			}
		} else if vr == "FD" || vr == "OD" {
			for !d.EOF() {
				data = append(data, d.ReadFloat64())
			}
		} else {
			// List of strings, each delimited by '\\'.
			v := d.ReadString(int(vl))
			// String may have '\0' suffix if its length is odd.
			str := strings.Trim(v, " \000")
			if len(str) > 0 {
				for _, s := range strings.Split(str, "\\") {
					data = append(data, s)
				}
			}
		}
	}
	elem.Value = data
	return elem
}

func readTag(buffer *dicomio.Decoder) dicomtag.Tag {

	group := buffer.ReadUInt16()

	element := buffer.ReadUInt16()

	return dicomtag.Tag{group, element}
}

// 从DICOM字典中读取VR，VL是32比特无符号数字
func readImplicit(buffer *dicomio.Decoder, tag dicomtag.Tag) (string, uint32) {

	vr := "UN"
	if entry, err := dicomtag.Find(tag); err == nil {
		vr = entry.VR
	}

	vl := buffer.ReadUInt32()
	if vl != UndefinedLength && vl%2 != 0 {
		buffer.SetErrorf("Encountered odd length (vl=%v) when reading implicit VR '%v' for tag %s", vl, vr, dicomtag.DebugString(tag))
		vl = 0
	}

	return vr, vl
}

// VR由下两个连续的bytes代表
// VL根据VR的值
// PS3.5 7.1.2
func readExplicit(buffer *dicomio.Decoder, tag dicomtag.Tag) (string, uint32) {

	vr := buffer.ReadString(2)
	var vl uint32

	switch vr {
	// TODO 下列情况与 PS3.5的7.1.1有区别
	// (http://dicom.nema.org/Dicom/2013/output/chtml/part05/chapter_7.html#table_7.1-1).
	case "NA", "OB", "OD", "OF", "OL", "OW", "SQ", "UN", "UC", "UR", "UT":
		buffer.Skip(2) // 忽略两个bytes，给未来用(0000H)
		vl = buffer.ReadUInt32()
		if vl == UndefinedLength && (vr == "UC" || vr == "UR" || vr == "VI") {
			buffer.SetError(errors.New("UC, UR 和 UT 也许没有一个未定义的长度(may not have an undefined length), 如值FFFFFFFFH的长度"))
			vl = 0
		}
	default:
		vl = uint32(buffer.ReadUInt16())
		// 纠正未定义的vl
		if vl == 0xffff {
			vl = UndefinedLength
		}
	}

	if vl != UndefinedLength && vl%2 != 0 {
		buffer.SetErrorf("Encountered odd length (vl=%v) when reading explicit VR %v for tag %s", vl, vr, dicomtag.DebugString(tag))
		vl = 0
	}

	return vr, vl
}

// ReadDataSet用io读取dicom file
// 当读取错误时，这个函数可能会返回部分可读取文件和读取时发现的第一个错误
func ReadDataSet(in io.Reader, options ReadOptions) (*DataSet, error) {

	buffer := dicomio.NewDecoder(in, binary.LittleEndian, dicomio.ExplicitVR)

	metaElements := ParseFileHeader(buffer)

	if buffer.Error() != nil {
		return nil, buffer.Error()
	}

	file := &DataSet{Elements: metaElements}

	// 改变剩余文件的 transfer syntax
	endian, implicit, err := getTransferSyntax(file)
	if err != nil {
		return nil, err
	}

	buffer.PushTransferSyntax(endian, implicit)
	defer buffer.PopTransferSyntax()

	// 读取elements数组
	for !buffer.EOF() {
		startLen := buffer.BytesRead()

		elem := ReadElement(buffer, options)

		if buffer.BytesRead() <= startLen { // 避免无限循环
			panic(fmt.Sprintf("ReadElement 读取data失败：position：%d: %v", startLen, buffer.Error()))
		}

		if elem == endOfDataElement {
			// element 是一个被options丢弃的pixel data
			break
		}

		if elem == nil {
			// 读取错误
			continue
		}

		if elem.Tag == dicomtag.SpecificCharacterSet {
			// 将剩余文件设为[]byte -> string decoder
			// It's sad that SpecificCharacterSet isn't part
			// of metadata, but is part of regular attrs, so we need
			// to watch out for multiple occurrences of this type of
			// elements.
			encodingNames, err := elem.GetStrings()
			if err != nil {
				buffer.SetError(err)
			} else {
				// SpecificCharacterSet 也许会出现在一个SQ/NA中，在这种情况下,
				// 这个charset是被固定在SQ/NA内，所以我们需要一个stack来记录？这些charset
				cs, err := dicomio.ParseSpecificCharacterSet(encodingNames)
				if err != nil {
					buffer.SetError(err)
				} else {
					buffer.SetCodingSystem(cs)
				}
			}
		}

		if options.ReturnTags == nil || (options.ReturnTags != nil && tagInList(elem.Tag, options.ReturnTags)) {
			file.Elements = append(file.Elements, elem)
		}
	}
	return file, buffer.Error()
}

func ReadDataSetInBytes(data []byte, options ReadOptions) (*DataSet, error) {
	return ReadDataSet(bytes.NewReader(data), options)
}

func getTransferSyntax(ds *DataSet) (byteorder binary.ByteOrder, implicit dicomio.IsImplicitVR, err error) {

	elem, err := ds.FindElementByTag(dicomtag.TransferSyntaxUID)
	if err != nil {
		return nil, dicomio.UnknownVR, err
	}

	transferSyntaxUID, err := elem.GetString()
	if err != nil {
		return nil, dicomio.UnknownVR, err
	}

	return dicomio.ParseTransferSyntaxUID(transferSyntaxUID)
}

// ReadDataSetFromFile 读取文件内容到 element.DataSet. 是一层ReadDataSet的包装
// 如果读取失败，会返回一个非空dataset和一个非空error，当出现错误时
// dataset会包含一部分可以读取的文件，error里会包含读取时的第一个错误
func ReadDataSetFromFile(path string, options ReadOptions) (*DataSet, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	ds, err := ReadDataSet(file, options)
	if e := file.Close(); e != nil && err == nil {
		err = e
	}

	return ds, err
}

func tagInList(tag dicomtag.Tag, tags []dicomtag.Tag) bool {
	for _, t := range tags {
		if tag == t {
			return true
		}
	}
	return false
}

// FindElementByName 寻找指定name的element
// 如“PatientName”
func (f *DataSet) FindElementByName(name string) (*Element, error) {

	return FindElementByName(f.Elements, name)
}

// FindElementByTag finds an element from the dataset given its tag, such as
// Tag{0x0010, 0x0010}.
func (f *DataSet) FindElementByTag(tag dicomtag.Tag) (*Element, error) {
	return FindElementByTag(f.Elements, tag)
}

// FindElementBuyName finds an element with the given Element.Name in
// "elements" If not found, return an error
func FindElementByName(elems []*Element, name string) (*Element, error) {
	t, err := dicomtag.FindByName(name)
	if err != nil {
		return nil, err
	}

	for _, elem := range elems {
		if elem.Tag == t.Tag {
			return elem, nil
		}
	}

	return nil, fmt.Errorf("could not find element named '%s' in dicom file", name)
}

// FindElementByTag finds an element with the given Element.Tag in
// "elements" If not found, returns an error.
func FindElementByTag(elems []*Element, tag dicomtag.Tag) (*Element, error) {

	for _, elem := range elems {
		if elem.Tag == tag {
			return elem, nil
		}
	}

	return nil, fmt.Errorf("%s: element not found", dicomtag.DebugString(tag))
}
