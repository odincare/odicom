package dicomio

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
)

// CodingSystem 定义了[]byte如何转译为utf-8字符串
type CodingSystem struct {
	// VR = "PN" 只在可能用到三个解码器时被替换
	// 对于所有的VR格式，只有Ideographic docoder被使用
	// 详情见 p3.5, 6.2
	//
	// 作者说p3.5 6.1本应该定义coding system的细节，但是他看不懂。
	// 所以直接从pydicom的charset.py复制了一份
	Alphabetic  *encoding.Decoder
	Ideographic *encoding.Decoder
	Phonetic    *encoding.Decoder
}

// CodingSystemType定义了哪一个coding system将会被使用，这个区别在日语中好用，但在其他语言不好用 = =
type CodingSystemType int

const (
	// AlphabeticCodingSystem is for writing a name in (English) alphabets.
	AlphabeticCodingSystem CodingSystemType = iota
	// IdeographicCodingSystem is for writing the name in the native writing
	// system (Kanji).
	IdeographicCodingSystem
	// PhoneticCodingSystem is for hirakana and/or katakana.
	PhoneticCodingSystem
)

// Mapping DICOM charset name to golang encoding/htmlindex name.  "" 为 7bit ascii.
var htmlEncodingNames = map[string]string{
	"ISO 2022 IR 6":   "iso-8859-1",
	"ISO_IR 13":       "shift_jis",
	"ISO 2022 IR 13":  "shift_jis",
	"ISO_IR 100":      "iso-8859-1",
	"ISO 2022 IR 100": "iso-8859-1",
	"ISO_IR 101":      "iso-8859-2",
	"ISO 2022 IR 101": "iso-8859-2",
	"ISO_IR 109":      "iso-8859-3",
	"ISO 2022 IR 109": "iso-8859-3",
	"ISO_IR 110":      "iso-8859-4",
	"ISO 2022 IR 110": "iso-8859-4",
	"ISO_IR 126":      "iso-ir-126",
	"ISO 2022 IR 126": "iso-ir-126",
	"ISO_IR 127":      "iso-ir-127",
	"ISO 2022 IR 127": "iso-ir-127",
	"ISO_IR 138":      "iso-ir-138",
	"ISO 2022 IR 138": "iso-ir-138",
	"ISO_IR 144":      "iso-ir-144",
	"ISO 2022 IR 144": "iso-ir-144",
	"ISO_IR 148":      "iso-ir-148",
	"ISO 2022 IR 148": "iso-ir-148",
	"ISO 2022 IR 149": "euc-kr",
	"ISO 2022 IR 159": "iso-2022-jp",
	"ISO_IR 166":      "iso-ir-166",
	"ISO 2022 IR 166": "iso-ir-166",
	"ISO 2022 IR 87":  "iso-2022-jp",
	"ISO_IR 192":      "utf-8",
	"GB18030":         "utf-8",
}

// ParseSpecificCharacterSet 覆盖DICOM character的编码名，
// 如”ISO-IR 100“ 用golang的解码器解码会为nil， nil是（7比特ASCII解码的）默认值
// 详情见 Cf. p3.2
// D.6.2  http://dicom.nema.org/medical/dicom/2016d/output/chtml/part02/sect_D.6.2.html
func ParseSpecificCharacterSet(encodingNames []string) (CodingSystem, error) {
	// 将剩余文件设为[]byte->string decoder
	// It's sad that SpecificCharacterSet isn't part
	// of metadata, but is part of regular attrs, so we need
	// to watch out for multiple occurrences of this type of
	// elements.
	//
	// encodingNames, err := elem.GetString()
	// if err != nil {
	// return CodingSystem{}, err
	// }
	var decoders []*encoding.Decoder

	for _, name := range encodingNames {
		var c *encoding.Decoder
		logrus.Warnf("io.ParseSpecificCharacterSet: Using coding system %s", name)

		if htmlName, ok := htmlEncodingNames[name]; !ok {
			// TODO 支持更多encodings
			return CodingSystem{}, fmt.Errorf("io.ParseSpecificCharacterSet: Unknown character set '%s'. Assuming utf-8", encodingNames[0])
		} else {
			if htmlName != "" {
				d, err := htmlindex.Get(htmlName)
				if err != nil {
					logrus.Panic(fmt.Sprintf("Encoding name %s (for %s) not found", name, htmlName))
				}

				c = d.NewDecoder()
			}
		}

		decoders = append(decoders, c)
	}

	if len(decoders) == 0 {
		return CodingSystem{nil, nil, nil}, nil
	}

	if len(decoders) == 1 {
		return CodingSystem{decoders[0], decoders[0], decoders[0]}, nil
	}

	if len(decoders) == 2 {
		return CodingSystem{decoders[0], decoders[1], decoders[1]}, nil
	}

	return CodingSystem{decoders[0], decoders[1], decoders[2]}, nil
}
