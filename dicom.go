package dicom

// TODO 这个好像会为每一个经手的dcm文件写入标志标识是go dicom修改 这里可以去申请一个新的
// GoDICOMImplementationClassUIDPrefix defines the UID prefix for
// go-dicom. Provided by https://www.medicalconnections.co.uk/Free_UID
const GoDICOMImplementationClassUIDPrefix = "1.2.826.0.1.3680043.9.7133"

var GoDICOMImplementationClassUID = GoDICOMImplementationClassUIDPrefix + ".1.1"

const GoDICOMImplementationVersionName = "GODICOM_1_1"
