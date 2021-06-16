package dicomtag

import (
	"fmt"
	"strconv"
	"strings"
)

// Tag 是一个定义了dicom文件中element 的类型的 <group, element> 元组
// 列表中的标准tags定义在tag_definitions.go, 也可以参考：
// ftp://medical.nema.org/medical/dicom/2011/11_06pu.pdf
type Tag struct {
	// Group 和 Element 是读取16进制对的结果 如 (1000,10008)
	Group   uint16
	Element uint16
}

// Compare 返回 -1/0/1 如果t<other | t==other | t>other，
// tag先由group排序，再由element排序
func (t Tag) Compare(other Tag) int {
	if t.Group < other.Group {
		return -1
	}

	if t.Group > other.Group {
		return 1
	}

	if t.Element < other.Element {
		return -1
	}

	if t.Element > other.Element {
		return 1
	}

	return 0
}

func IsPrivate(group uint16) bool {
	return group%2 == 1
}

// String 返回一个如"(0008, 1234)"格式的string
// 0x0008 是 t.Group 0x1234是t.Element
func (t Tag) String() string {
	return fmt.Sprintf("(%04x, %04x)", t.Group, t.Element)
}

// TagInfo 保存了Tag在标准DICOM标准中的detail information
type TagInfo struct {
	Tag Tag
	// Data 编码 如 "UL" "CS"
	VR string
	// 人类可读的Tag名称 如 "CommandDataSetType"
	Name string
	// 基数(Cardinality) (element中期望的值 #)
	VM string
}

// MetadataGroup 是 Tag.Group 中 metadata tags的值.
const MetadataGroup = 2

// VRKind 定义了golang 编码的VR
type VRKind int

const (
	// VRStringList means the element stores a list of strings
	VRStringList VRKind = iota
	// VRBytes means the element stores a []byte
	VRBytes
	// VRString means the element stores a string
	VRString
	// VRUInt16List means the element stores a list of uint16s
	VRUInt16List
	// VRUInt32List means the element stores a list of uint32s
	VRUInt32List
	// VRInt16List means the element stores a list of int16s
	VRInt16List
	// VRInt32List element stores a list of int32s
	VRInt32List
	// VRFloat32List element stores a list of float32s
	VRFloat32List
	// VRFloat64List element stores a list of float64s
	VRFloat64List
	// VRSequence means the element stores a list of *Elements, w/ TagItem
	VRSequence
	// VRItem means the element stores a list of *Elements
	VRItem
	// VRTagList element stores a list of Tags
	VRTagList
	// VRDate means the element stores a date string. Use ParseDate() to
	// parse the date string.
	VRDate
	// VRPixelData means the element stores a PixelDataInfo
	VRPixelData
)

// GetVRKind 返回 go语言的 value encoding of an element with <tag, vr>.
func GetVRKind(tag Tag, vr string) VRKind {
	if tag == Item {
		return VRItem
	} else if tag == PixelData {
		return VRPixelData
	}
	switch vr {
	case "DA":
		return VRDate
	case "AT":
		return VRTagList
	case "OW", "OB":
		return VRBytes
	case "LT", "UT":
		return VRString
	case "UL":
		return VRUInt32List
	case "SL":
		return VRInt32List
	case "US":
		return VRUInt16List
	case "SS":
		return VRInt16List
	case "FL":
		return VRFloat32List
	case "FD":
		return VRFloat64List
	case "SQ":
		return VRSequence
	default:
		return VRStringList
	}
}

// 找到给与的tag中的信息
// 如果tag不是dicom standard的一部分或已经不再在dicom standard中 会返回错误
func Find(tag Tag) (TagInfo, error) {
	maybeInitTagDict()
	entry, ok := tagDict[tag]
	if !ok {
		// (0000-u-ffff,0000)	UL	GenericGroupLength	1	GENERIC
		if tag.Group%2 == 0 && tag.Element == 0x0000 {
			entry = TagInfo{tag, "UL", "GenericGroupLength", "1"}
		} else {
			return TagInfo{}, fmt.Errorf("Could not find tag (0x%x, 0x%x) in dictionary", tag.Group, tag.Element)
		}
	}
	return entry, nil
}

// MustFind与FindTag相似, 但报错会panic停止程序
func MustFind(tag Tag) TagInfo {
	e, err := Find(tag)
	if err != nil {
		panic(fmt.Sprintf("tag %v not found: %s", tag, err))
	}
	return e
}

// FindByName将传入的name寻找到information。
// 如果tag不是dicom standard中的一个或者不再在dicom standard中，将会返回一个错误
// 例: FindTagByName("TransferSyntaxUID")
func FindByName(name string) (TagInfo, error) {
	maybeInitTagDict()
	for _, ent := range tagDict {
		if ent.Name == name {
			return ent, nil
		}
	}
	return TagInfo{}, fmt.Errorf("could not find tag with name %s", name)
}

// DebugString 返回一个人类可读的tag的诊断字符串，格式如 "(group, element)[name]"
func DebugString(tag Tag) string {
	e, err := Find(tag)
	if err != nil {
		if IsPrivate(tag.Group) {
			return fmt.Sprintf("(%04x,%04x)[private]", tag.Group, tag.Element)
		} else {
			return fmt.Sprintf("(%04x,%04x)[??]", tag.Group, tag.Element)
		}
	}
	return fmt.Sprintf("(%04x,%04x)[%s]", tag.Group, tag.Element, e.Name)
}

// 将tag分成 group和element 由16进制数表示
// TODO: support group ranges (6000-60FF,0803)
func parseTag(tag string) (Tag, error) {
	parts := strings.Split(strings.Trim(tag, "()"), ",")
	group, err := strconv.ParseInt(parts[0], 16, 0)
	if err != nil {
		return Tag{}, err
	}
	elem, err := strconv.ParseInt(parts[1], 16, 0)
	if err != nil {
		return Tag{}, err
	}
	return Tag{Group: uint16(group), Element: uint16(elem)}, nil
}
