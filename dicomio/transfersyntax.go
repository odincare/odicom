package dicomio

import (
	"encoding/binary"
	"fmt"
	"github.com/odincare/odicom/dicomuid"
)

// StandardTransferSyntaxes is the list of standard transfer syntaxes
var StandardTransferSyntaxes = []string{
	dicomuid.ImplicitVRLittleEndian,
	dicomuid.ExplicitVRLittleEndian,
	dicomuid.ExplicitVRBigEndian,
	dicomuid.DeflatedExplicitVRLittleEndian,
}

// CanonicalTransferSyntaxUID return the canonical transfer syntax UID
// (e.g. uid.ExplicitVRLittleEndian or uid.ImplicitVrLittleEndian),
// given an UID that represents any transfer syntax. Returns an error if
// the uid is not defined in DICOM standard, or if the uid does not represent
// a transfer syntax
// TODO check the standard to see if we need to accept unknown UIDS
// as explicit little endian.
func CanonicalTransferSyntaxUID(uid string) (string, error) {

	// defaults are explicit VR, little endian
	switch uid {
	case dicomuid.ImplicitVRLittleEndian,
		dicomuid.ExplicitVRLittleEndian,
		dicomuid.ExplicitVRBigEndian,
		dicomuid.DeflatedExplicitVRLittleEndian:
		return uid, nil
	default:
		e, err := dicomuid.Lookup(uid)
		if err != nil {
			return "", nil
		}

		if e.Type != dicomuid.TypeTransferSyntax {
			return "", fmt.Errorf("dicom.CanonicalTransferSyntaxUID: '%s' is not a transfer syntax (is %s)", uid, e.Type)
		}

		// the default is ExplicitVRLittleEndian
		return dicomuid.ExplicitVRLittleEndian, nil
	}
}

// ParseTransferSyntaxUID parses a transfer syntax uid and returns its byteorder
// and implicitVR/explicitVR type. TransferSyntaxUID can be any UID that refers to
// a transfer syntax. It can be, e.g.
// 1.2.840.1008.1.2(it will return (LittleEndian, ImplicitVR))
// or 1.2.840.1008.1.2.4.54(it will return (LittleEndian, ExplicitVR))
func ParseTransferSyntaxUID(uid string) (byteorder binary.ByteOrder, implicit IsImplicitVR, err error) {

	canonical, err := CanonicalTransferSyntaxUID(uid)
	if err != nil {
		return nil, UnknownVR, err
	}

	switch canonical {
	case dicomuid.ImplicitVRLittleEndian:
		return binary.LittleEndian, ImplicitVR, nil
	case dicomuid.DeflatedExplicitVRLittleEndian:
		fallthrough
	case dicomuid.ExplicitVRLittleEndian:
		return binary.LittleEndian, ExplicitVR, nil
	case dicomuid.ExplicitVRBigEndian:
		return binary.BigEndian, ExplicitVR, nil
	default:
		panic(fmt.Sprintf("Invalid transfer syntax: %v, %v", canonical, uid))
	}
}
