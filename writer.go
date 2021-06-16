package dicom

import (
	"encoding/binary"
	"fmt"
	"io"
	"odicom/dicomio"
	"odicom/dicomtag"
	"os"

	"github.com/sirupsen/logrus"
)

// WriteFileHeader produces a Dicom file header. metaElements[] is be a list of
// elements to be embedded in the header part. Every element in metaElements[]
// must have Tag.Group==2. It must contain at least the following three elements:
// TagTransferSyntaxUID, TagMediaStorageSOPClassUID, TagMediaStorageSOPInstanceUID.
// The list may contain other meta elements as long as their Tag.Group==2;
// they are added to the header
//
// Errors are reported via e.Error().
//
// Consult the following page for the Dicom file header format
// http://dicom.nema.org/dicom/2013/output/chtml/part10/chapter_7.html
func WriteFileHeader(e *dicomio.Encoder, metaElements []*Element) {

	e.PushTransferSyntax(binary.LittleEndian, dicomio.ExplicitVR)
	defer e.PopTransferSyntax()

	subEncoder := dicomio.NewBytesEncoder(binary.LittleEndian, dicomio.ExplicitVR)

	tagsUsed := make(map[dicomtag.Tag]bool)

	tagsUsed[dicomtag.FileMetaInformationGroupLength] = true

	writeRequiredMetaElement := func(tag dicomtag.Tag) {
		if elem, err := FindElementByTag(metaElements, tag); err == nil {
			WriteElement(subEncoder, elem)
		} else {
			subEncoder.SetErrorf("%v not found in metaElements: %v", dicomtag.DebugString(tag), err)
		}

		tagsUsed[tag] = true
	}

	writeOptionalMetaElement := func(tag dicomtag.Tag, defaultValue interface{}) {
		if elem, err := FindElementByTag(metaElements, tag); err == nil {
			WriteElement(subEncoder, elem)
		} else {
			WriteElement(subEncoder, MustNewElement(tag, defaultValue))
		}

		tagsUsed[tag] = true
	}

	// TODO ?
	writeOptionalMetaElement(dicomtag.FileMetaInformationVersion, []byte("0 1"))
	writeRequiredMetaElement(dicomtag.MediaStorageSOPClassUID)
	writeRequiredMetaElement(dicomtag.MediaStorageSOPInstanceUID)
	writeRequiredMetaElement(dicomtag.TransferSyntaxUID)
	writeOptionalMetaElement(dicomtag.ImplementationClassUID, GoDICOMImplementationClassUID)
	writeOptionalMetaElement(dicomtag.ImplementationVersionName, GoDICOMImplementationVersionName)

	for _, elem := range metaElements {
		if elem.Tag.Group == dicomtag.MetadataGroup {
			if _, ok := tagsUsed[elem.Tag]; !ok {
				WriteElement(subEncoder, elem)
			}
		}
	}

	if subEncoder.Error() != nil {
		e.SetError(subEncoder.Error())
		return
	}

	metaBytes := subEncoder.Bytes()

	e.WriteZeros(128)
	e.WriteString("DICM")

	WriteElement(e, MustNewElement(dicomtag.FileMetaInformationGroupLength, uint32(len(metaBytes))))

	e.WriteBytes(metaBytes)
}

func writeRawItem(e *dicomio.Encoder, data []byte) {
	encodeElementHeader(e, dicomtag.Item, "NA", uint32(len(data)))
	e.WriteBytes(data)
}

func writeBasicOffsetTable(e *dicomio.Encoder, offsets []uint32) {

	byteOrder, _ := e.TransferSyntax()

	subEncoder := dicomio.NewBytesEncoder(byteOrder, dicomio.ImplicitVR)
	for _, offset := range offsets {
		subEncoder.WriteUInt32(offset)
	}

	writeRawItem(e, subEncoder.Bytes())
}

func encodeElementHeader(e *dicomio.Encoder, tag dicomtag.Tag, vr string, vl uint32) {
	// TODO ??? 这里是what
	dicomio.DoAssert(vl == UndefinedLength || vl%2 == 0, vl)

	e.WriteUInt16(tag.Group)
	e.WriteUInt16(tag.Element)

	_, implicit := e.TransferSyntax()
	if tag.Group == ItemSeqGroup {
		implicit = dicomio.ImplicitVR
	}

	if implicit == dicomio.ExplicitVR {
		dicomio.DoAssert(len(vr) == 2, vr)
		e.WriteString(vr)

		switch vr {
		case "NA", "OB", "OD", "OF", "OL", "OW", "SQ", "UN", "UC", "UR", "UT":
			e.WriteZeros(2) // 2 bytes for "future use" (0000H)
			e.WriteUInt32(vl)
		default:
			e.WriteUInt16(uint16(vl))
		}
	} else {
		dicomio.DoAssert(implicit == dicomio.ImplicitVR, implicit)
		e.WriteUInt32(vl)
	}
}

// WriteElement encodes one data element, Errors are reported through e.Error()
// and/or E.Finish().
//
// Requires: Each value in values[] must match the VR of the tag.
// e.g. if tag is for UL, then each value must be uint32
func WriteElement(e *dicomio.Encoder, elem *Element) {

	vr := elem.VR

	entry, err := dicomtag.Find(elem.Tag)

	if vr == "" {
		if err == nil {
			vr = entry.VR
		} else {
			vr = "UN"
		}
	} else {
		if err == nil && entry.VR != vr {
			if dicomtag.GetVRKind(elem.Tag, entry.VR) != dicomtag.GetVRKind(elem.Tag, vr) {
				// The golang repl, is different. We can't continue
				e.SetErrorf("dicom.WriteElement: VR value dismatch for tag %s. Element.VR=%v, but Dicom standard defines VR to be %v",
					dicomtag.DebugString(elem.Tag), vr, entry.VR)
				return
			}
			logrus.Warnf("dicom.WriteElement: VR value mismatch for tag %s. Element.VR=%v, but DICOM standard defines VR to be %v (continuing)",
				dicomtag.DebugString(elem.Tag), vr, entry.VR)
		}
	}

	dicomio.DoAssert(vr != "", vr)

	if elem.Tag == dicomtag.PixelData {
		if len(elem.Value) != 1 {
			// TODO 暂时用PixelDataInfo()
			e.SetError(fmt.Errorf("PixelData element must have one value of type PixelDataInfo"))
		}

		image, ok := elem.Value[0].(PixelDataInfo)
		if !ok {
			e.SetError(fmt.Errorf("PixelData的子元素的类型必须是PixelDataInfo"))
		}

		if elem.UndefinedLength {
			encodeElementHeader(e, elem.Tag, vr, UndefinedLength)
			writeBasicOffsetTable(e, image.Offsets)

			for _, image := range image.Frames {
				writeRawItem(e, image)
			}

			encodeElementHeader(e, dicomtag.SequenceDelimitationItem, "" /*未使用*/, 0)
		} else {
			dicomio.DoAssert(len(image.Frames) == 1, image.Frames) // TODO ?
			encodeElementHeader(e, elem.Tag, vr, uint32(len(image.Frames[0])))
			e.WriteBytes(image.Frames[0])
		}

		return
	}

	if vr == "SQ" {
		if elem.UndefinedLength {
			encodeElementHeader(e, elem.Tag, vr, UndefinedLength)

			for _, value := range elem.Value {
				subelem, ok := value.(*Element)
				if !ok || subelem.Tag != dicomtag.Item {
					e.SetError(fmt.Errorf("SQ element 必须是一个Item, 而不是：%v", value))
					return
				}

				WriteElement(e, subelem)
			}

			encodeElementHeader(e, dicomtag.SequenceDelimitationItem, "" /*未使用*/, 0)
		} else {

			sube := dicomio.NewBytesEncoder(e.TransferSyntax())

			for _, value := range elem.Value {
				subelem, ok := value.(*Element)
				if !ok || subelem.Tag != dicomtag.Item {
					e.SetError(fmt.Errorf("SQ element 必须是一个Item, 而不是：%v", value))
					return
				}

				WriteElement(sube, subelem)
			}

			if sube.Error() != nil {
				e.SetError(sube.Error())
				return
			}

			bytes := sube.Bytes()

			encodeElementHeader(e, elem.Tag, vr, uint32(len(bytes)))

			e.WriteBytes(bytes)
		}
	} else if vr == "NA" { // item

		if elem.UndefinedLength {
			encodeElementHeader(e, elem.Tag, vr, UndefinedLength)

			for _, value := range elem.Value {
				subelem, ok := value.(*Element)

				if !ok {
					e.SetErrorf("Item values 必须是一个 dicom.Element, 而不是: %v", value)
					return
				}

				WriteElement(e, subelem)
			}

			encodeElementHeader(e, dicomtag.ItemDelimitationItem, "" /*未使用*/, 0)
		} else {
			sube := dicomio.NewBytesEncoder(e.TransferSyntax())

			for _, value := range elem.Value {

				subelem, ok := value.(*Element)
				if !ok {
					e.SetErrorf("Item values 必须是一个 dicom.Element, 而不是: %v", value)
					return
				}

				WriteElement(sube, subelem)
			}

			if sube.Error() != nil {
				e.SetError(sube.Error())
				return
			}

			bytes := sube.Bytes()
			encodeElementHeader(e, elem.Tag, vr, uint32(len(bytes)))
			e.WriteBytes(bytes)
		}
	} else {
		if elem.UndefinedLength {
			e.SetErrorf("目前还不支持编码undefined-length的element: %v", elem)
			return
		}

		sube := dicomio.NewBytesEncoder(e.TransferSyntax())

		switch vr {
		case "US":
			for _, value := range elem.Value {

				v, ok := value.(uint16)
				if !ok {
					e.SetErrorf("%v: 需要是uint16类型，而不是: %v",
						dicomtag.DebugString(elem.Tag), value)
					continue
				}

				sube.WriteUInt16(v)
			}
		case "UL":
			for _, value := range elem.Value {
				v, ok := value.(uint32)
				if !ok {
					e.SetErrorf("%v: 需要是uint32类型, 而不是: %v",
						dicomtag.DebugString(elem.Tag), value)
					continue
				}
				sube.WriteUInt32(v)
			}
		case "SL":
			for _, value := range elem.Value {
				v, ok := value.(int32)
				if !ok {
					e.SetErrorf("%v: 需要是int32类型, 而不是: %v",
						dicomtag.DebugString(elem.Tag), value)
					continue
				}
				sube.WriteInt32(v)
			}
		case "SS":
			for _, value := range elem.Value {
				v, ok := value.(int16)
				if !ok {
					e.SetErrorf("%v: 需要是int16类型, 而不是: %v",
						dicomtag.DebugString(elem.Tag), value)
					continue
				}
				sube.WriteInt16(v)
			}
		case "FL":
			fallthrough
		case "OF":
			for _, value := range elem.Value {
				v, ok := value.(float32)
				if !ok {
					e.SetErrorf("%v: 需要是float32类型, 而不是: %v",
						dicomtag.DebugString(elem.Tag), value)
					continue
				}
				sube.WriteFloat32(v)
			}
		case "FD":
			fallthrough
		case "OD":
			for _, value := range elem.Value {
				v, ok := value.(float64)
				if !ok {
					e.SetErrorf("%v: 需要是float64类型, 而不是: %v",
						dicomtag.DebugString(elem.Tag), value)
					continue
				}
				sube.WriteFloat64(v)
			}
		case "OW", "OB": // TODO 检查大小是不是均衡（even）. Byte swap??
			if len(elem.Value) != 1 {
				e.SetErrorf("%v: 需要单个value, 而不是: %v",
					dicomtag.DebugString(elem.Tag), elem.Value)
				break
			}
			bytes, ok := elem.Value[0].([]byte)
			if !ok {
				e.SetErrorf("%v: 需要一个二进制字符串，而不是: %v",
					dicomtag.DebugString(elem.Tag), elem.Value[0])
				break
			}
			if vr == "OW" {
				if len(bytes)%2 != 0 {
					e.SetErrorf("%v: 需要一个长度均匀（even length）的二进制字符串, 而不是长度（length） %v",
						dicomtag.DebugString(elem.Tag), len(bytes))
					break
				}
				d := dicomio.NewBytesDecoder(bytes, dicomio.NativeByteOrder, dicomio.UnknownVR)
				n := len(bytes) / 2
				for i := 0; i < n; i++ {
					v := d.ReadUInt16()
					sube.WriteUInt16(v)
				}
				dicomio.DoAssert(d.Finish() == nil, d.Error())
			} else { // vr=="OB"
				sube.WriteBytes(bytes)
				if len(bytes)%2 == 1 {
					sube.WriteByte(0)
				}
			}
		case "UI":
			s := ""
			for i, value := range elem.Value {
				substr, ok := value.(string)
				if !ok {
					e.SetErrorf("%v: 非字符串的值", dicomtag.DebugString(elem.Tag))
					continue
				}
				if i > 0 {
					s += "\\"
				}
				s += substr
			}
			sube.WriteString(s)
			if len(s)%2 == 1 {
				sube.WriteByte(0)
			}
		case "AT", "NA":
			fallthrough
		default:
			s := ""
			for i, value := range elem.Value {
				substr, ok := value.(string)
				if !ok {
					e.SetErrorf("%v: 非字符串的值", dicomtag.DebugString(elem.Tag))
					continue
				}
				if i > 0 {
					s += "\\"
				}
				s += substr
			}
			sube.WriteString(s)
			if len(s)%2 == 1 {
				sube.WriteByte(' ')
			}
		}

		if sube.Error() != nil {
			e.SetError(sube.Error())
			return
		}

		bytes := sube.Bytes()
		encodeElementHeader(e, elem.Tag, vr, uint32(len(bytes)))
		e.WriteBytes(bytes)
	}
}

// WriteDataSet writes the dataset into the stream in DICOM file format,
// complete with the magic header and metadata elements.
//
// The transfer syntax (byte order, etc) of the file is determined by the
// TransferSyntax element in "ds". If ds is missing that or a few other
// essential elements, this function returns an error.
//
//  ds := ... read or create dicom.Dataset ...
//  out, err := os.Create("test.dcm")
//  err := dicom.Write(out, ds)
func WriteDataSet(out io.Writer, ds *DataSet) error {
	e := dicomio.NewEncoder(out, nil, dicomio.UnknownVR)
	var metaElems []*Element
	for _, elem := range ds.Elements {
		if elem.Tag.Group == dicomtag.MetadataGroup {
			metaElems = append(metaElems, elem)
		}
	}
	WriteFileHeader(e, metaElems)
	if e.Error() != nil {
		return e.Error()
	}
	endian, implicit, err := getTransferSyntax(ds)
	if err != nil {
		return err
	}
	e.PushTransferSyntax(endian, implicit)
	for _, elem := range ds.Elements {
		if elem.Tag.Group != dicomtag.MetadataGroup {
			WriteElement(e, elem)
		}
	}
	e.PopTransferSyntax()
	return e.Error()
}

// WriteDataSetToFile writes "ds" to the given file. If the file already exists,
// existing contents are clobbered. Else, the file is newly created.
func WriteDataSetToFile(path string, ds *DataSet) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := WriteDataSet(out, ds); err != nil {
		out.Close() // nolint: errcheck
		return err
	}
	return out.Close()
}
