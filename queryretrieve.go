package dicom

import (
	"fmt"
	"github.com/odincare/odicom/dicomtag"

	"github.com/gobwas/glob"
	"github.com/sirupsen/logrus"
)

// 查询检查dataset是否符合QR condition "filter"。
// 如果是，就返回<true, 匹配的element, nil>
// 如果 "filter" 要求一个通用匹配(universal match) i.e. 空查询 empty query value 且 element的filter.Tag不存在，函数返回<true, nil, nil>
// 如果”filter“有误(malformed)，函数返回<false, nil, err reason>
func Query(ds *DataSet, f *Element) (match bool, matchedElement *Element, err error) {

	if len(f.Value) > 1 {
		// 过滤器不能包含多个值 P3.4 C2.2.2.1
		return false, nil, fmt.Errorf("multiple values found in filter '%v'", f)
	}

	if f.Tag == dicomtag.QueryRetrieveLevel || f.Tag == dicomtag.SpecificCharacterSet {
		return true, nil, nil
	}

	elem, err := ds.FindElementByTag(f.Tag)

	if err != nil {
		elem = nil
	}

	match, err = queryElement(elem, f)

	if match {
		return true, elem, nil
	}

	return false, nil, err
}

func queryElement(elem *Element, f *Element) (match bool, err error) {

	if isEmptyQuery(f) {
		// 通用匹配 一个空格代表通配符
		return true, nil
	}

	if f.VR == "SQ" {
		return querySequence(f, elem)
	}

	if elem == nil {
		// TODO 这可能是错的，不应该区分不存在的element和空element
		return false, err
	}

	if f.VR != elem.VR {
		// 这个应该不会发生 但还是写上了
		return false, fmt.Errorf("VR mismatch: filter %v, value %v", f, elem)
	}

	if f.VR == "UI" {
		// 判断element的filter是否至少包含一个uid
		for _, expected := range f.Value {
			e := expected.(string)

			for _, value := range elem.Value {
				if value.(string) == e {
					return true, nil
				}
			}
		}

		return false, nil
	}

	// TODO 处理日期匹配
	switch v := f.Value[0].(type) {

	case int32:
		for _, value := range elem.Value {
			if v == value.(int32) {
				return true, nil
			}
		}

	case int16:
		for _, value := range elem.Value {
			if v == value.(int16) {
				return true, nil
			}
		}
	case uint32:
		for _, value := range elem.Value {
			if v == value.(uint32) {
				return true, nil
			}
		}
	case uint16:
		for _, value := range elem.Value {
			if v == value.(uint16) {
				return true, nil
			}
		}
	case float32:
		for _, value := range elem.Value {
			if v == value.(float32) {
				return true, nil
			}
		}
	case float64:
		for _, value := range elem.Value {
			if v == value.(float64) {
				return true, nil
			}
		}
	case string:
		for _, value := range elem.Value {
			ok, err := matchString(v, value.(string))
			if err != nil {
				return false, err
			}

			return ok, nil
		}
	default:
		return false, errors.New(fmt.Sprintf("Unknown data: %v", f))
	}

	return false, nil
}

func querySequence(elem *Element, f *Element) (match bool, err error) {
	// TODO 继承？（Implement）
	return true, nil
}

func matchString(pattern string, value string) (bool, error) {

	g, err := glob.Compile(pattern)
	if err != nil {
		return false, err
	}

	return g.Match(value), err

}

func isEmptyQuery(f *Element) bool {
	// 检查匹配格式是否是一串 “*”
	// "*" 与 空查询一样是通用匹配符 P3.4 C2.2.2.4
	isUniversalGlob := func(s string) bool {
		for i := 0; i < len(s); i++ {
			if s[i] != '*' {
				return false
			}
		}

		return true
	}

	if len(f.Value) == 0 {
		return true
	}

	switch dicomtag.GetVRKind(f.Tag, f.VR) {
	case dicomtag.VRBytes:
		if len(f.Value[0].([]byte)) == 0 {
			return true
		}

	case dicomtag.VRString, dicomtag.VRDate:
		pattern := f.Value[0].(string)
		if len(pattern) == 0 {
			return true
		}

		if isUniversalGlob(pattern) {
			return true
		}

	case dicomtag.VRStringList:
		pattern := f.Value[0].(string)
		if isUniversalGlob(pattern) {
			return true
		}
	}

	return false
}
