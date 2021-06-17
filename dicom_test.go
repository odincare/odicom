package dicom_test

import (
	"fmt"
	"github.com/odincare/odicom"
	"github.com/odincare/odicom/dicomtag"
	"github.com/stretchr/testify/require"
	"log"
	"testing"
)

func mustReadFile(path string, options dicom.ReadOptions) *dicom.DataSet {
	data, err := dicom.ReadDataSetFromFile(path, options)
	if err != nil {
		log.Panic(err)
	}
	return data
}
func Example_read() {
	ds, err := dicom.ReadDataSetFromFile("examples/IM-0001-0003.dcm", dicom.ReadOptions{})
	if err != nil {
		panic(err)
	}
	patientID, err := ds.FindElementByTag(dicomtag.PatientID)
	if err != nil {
		panic(err)
	}
	patientBirthDate, err := ds.FindElementByTag(dicomtag.PatientBirthDate)
	if err != nil {
		panic(err)
	}
	institutionName, err := ds.FindElementByTag(dicomtag.InstitutionName)
	if err != nil {
		panic(err)
	}
	fmt.Println("ID: " + patientID.String())
	fmt.Println("BirthDate: " + patientBirthDate.String())
	fmt.Println("InstitutionName: " + institutionName.String())
	// Output:
	// ID:  (0010,0020)[PatientID] LO  [7DkT2Tp]
	// BirthDate:  (0010,0030)[PatientBirthDate] DA  [19530828]
	//InstitutionName:  (0008,0080)[InstitutionName] LO  [UCLA  Medical Center]
}
func Example_updateExistingFile() {
	ds, err := dicom.ReadDataSetFromFile("examples/IM-0001-0003.dcm", dicom.ReadOptions{})
	if err != nil {
		panic(err)
	}
	patientID, err := ds.FindElementByTag(dicomtag.PatientID)
	if err != nil {
		panic(err)
	}
	patientID.Value = []interface{}{"Zhang San"}

	//buf := bytes.Buffer{}
	//if err := dicom.WriteDataSet(&buf, ds); err != nil {
	//	panic(err)
	//}
	//
	//ds2, err := dicom.ReadDataSet(&buf, dicom.ReadOptions{})
	//if err != nil {
	//	panic(err)
	//}
	if err := dicom.WriteDataSetToFile("examples/test_write.dcm", ds); err != nil {
		panic(err)
	}
	patientID, err = ds.FindElementByTag(dicomtag.PatientID)
	if err != nil {
		panic(err)
	}
	fmt.Println("ID: " + patientID.String())
	// Output:
	// ID:  (0010,0020)[PatientID] LO  [Zhang San]
}

// Test ReadOptions
func TestReadOptions(t *testing.T) {
	// Test Drop Pixel Data
	data := mustReadFile("examples/IM-0001-0001.dcm", dicom.ReadOptions{DropPixelData: true})
	_, err := data.FindElementByTag(dicomtag.PatientName)
	require.NoError(t, err)
	_, err = data.FindElementByTag(dicomtag.PixelData)
	require.Error(t, err)

	// Test Return Tags
	data = mustReadFile("examples/IM-0001-0001.dcm", dicom.ReadOptions{DropPixelData: true, ReturnTags: []dicomtag.Tag{dicomtag.StudyInstanceUID}})
	_, err = data.FindElementByTag(dicomtag.StudyInstanceUID)
	if err != nil {
		t.Error(err)
	}
	_, err = data.FindElementByTag(dicomtag.PatientName)
	if err == nil {
		t.Errorf("PatientName should not be present")
	}

	// Test Stop at Tag
	data = mustReadFile("examples/IM-0001-0001.dcm",
		dicom.ReadOptions{
			DropPixelData: true,
			// Study Instance UID Element tag is Tag{0x0020, 0x000D}
			StopAtTag: &dicomtag.StudyInstanceUID})
	_, err = data.FindElementByTag(dicomtag.PatientName) // Patient Name Element tag is Tag{0x0010, 0x0010}
	if err != nil {
		t.Error(err)
	}
	_, err = data.FindElementByTag(dicomtag.SeriesInstanceUID) // Series Instance UID Element tag is Tag{0x0020, 0x000E}
	if err == nil {
		t.Errorf("PatientName should not be present")
	}
}
