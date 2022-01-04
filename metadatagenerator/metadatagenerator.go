package metadatagenerator

import (
	"ds-agent/models"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
)

//ErrorMessageMetaData - file process error metadata
type ErrorMessageMetaData struct {
	ErrorMessage string `json:"errorMessage"`
}

//FileDownloadErrorMetaData - file download error meta data
type FileDownloadErrorMetaData struct {
	ErrorMessage   string `json:"errorMessage"`
	RemoteTxnID    string `json:"remoteTxnID"`
	DATASTACKTxnID string `json:"dataStackTxnID"`
}

//GenerateErrorMessageMetaData - generate error message metadata
func GenerateErrorMessageMetaData(message string) string {
	metaData := ErrorMessageMetaData{}
	metaData.ErrorMessage = message
	byteData, _ := json.Marshal(metaData)
	return string(byteData)
}

//GenerateFileDownloadErrorMetaData - generate file download error meta data
func GenerateFileDownloadErrorMetaData(errorMessage string, remoteTxnID string, dataStackTxnID string) string {
	metaData := FileDownloadErrorMetaData{}
	metaData.ErrorMessage = errorMessage
	metaData.DATASTACKTxnID = dataStackTxnID
	metaData.RemoteTxnID = remoteTxnID
	byteData, _ := json.Marshal(metaData)
	return string(byteData)
}

//GenerateFileDownloadedInteractionMetaData - generate file download interaction meta data
func GenerateFileDownloadedInteractionMetaData(outputDirectory []string, blockName string, size string, fileName string, sequenceNo string, structureID string, remoteTxnID string, dataStackTxnID string, checksum string, ipAddress string, macAddress string, encrypt string, successFlow string) string {
	metaData := models.InteractionMetadata{}
	for _, dir := range outputDirectory {
		path, _ := filepath.Abs(dir)
		metaData.OutputDirectory = append(metaData.OutputDirectory, path)
	}
	metaData.BlockName = blockName
	metaData.Size = size
	fileNameSplitArray := strings.Split(fileName, ".")
	metaData.FileSuffix = fileNameSplitArray[len(fileNameSplitArray)-1]
	metaData.OriginalFileName = fileName
	metaData.SequenceNo, _ = strconv.Atoi(sequenceNo)
	metaData.StructureID = structureID
	metaData.RemoteTxnID = remoteTxnID
	metaData.DataStackTxnID = dataStackTxnID
	metaData.Md5CheckSum = checksum
	metaData.IPAddress = ipAddress
	metaData.MACAddress = macAddress
	metaData.Encrypt = encrypt
	if successFlow == "true" {
		metaData.SuccessFlow = true
	} else {
		metaData.SuccessFlow = false
	}
	byteData, _ := json.Marshal(metaData)
	return string(byteData)
}

//GenerateFileUploadMetaData - generate file upload metadata
func GenerateFileUploadMetaData(originalFileName string, originalFilePath string, mirrorPath string, uniqueRemoteTransaction bool, uniqueFileName bool, uniqueCheckSum bool, structureID string, blockName string, inputDirectory string, newFileName string, newLocation string, md5Checksum string, remoteTxnID string, dataStackTxnID string, properties models.FlowWatcherProperties) string {
	metaData := models.FileUploadMetaData{}
	metaData.OriginalFileName = originalFileName
	metaData.OriginalFilePath = originalFilePath
	metaData.MirrorPath = mirrorPath
	metaData.UniqueRemoteTransaction = strconv.FormatBool(uniqueRemoteTransaction)
	metaData.UniqueFileName = strconv.FormatBool(uniqueFileName)
	metaData.UniqueChecksum = strconv.FormatBool(uniqueCheckSum)
	metaData.StructureID = structureID
	metaData.BlockName = blockName
	metaData.InputDirectory = inputDirectory
	metaData.NewFileName = newFileName
	metaData.NewLocation = newLocation
	metaData.Md5CheckSum = md5Checksum
	metaData.RemoteTxnID = remoteTxnID
	metaData.DataStackTxnID = dataStackTxnID
	metaData.FlowWatcherProperties = properties
	byteData, _ := json.Marshal(metaData)
	return string(byteData)
}

//GenerateFileUploadErrorMetaDataForLargeFileNameOrMaxFileSize - generate file upload error metadata for large file name or max size
func GenerateFileUploadErrorMetaDataForLargeFileNameOrMaxFileSize(originalFileName string, originalFilePath string, mirrorPath string, uniqueRemoteTransaction bool, uniqueFileName bool, uniqueCheckSum bool, structureID string, blockName string, inputDirectory string, newFileName string, newLocation string, md5Checksum string, remoteTxnID string, dataStackTxnID string, errorMessage string) string {
	metaData := models.FileUploadMetaData{}
	metaData.OriginalFileName = originalFileName
	metaData.OriginalFilePath = originalFilePath
	metaData.UniqueRemoteTransaction = strconv.FormatBool(uniqueRemoteTransaction)
	metaData.UniqueFileName = strconv.FormatBool(uniqueFileName)
	metaData.UniqueChecksum = strconv.FormatBool(uniqueCheckSum)
	metaData.StructureID = structureID
	metaData.BlockName = blockName
	metaData.InputDirectory = inputDirectory
	metaData.NewFileName = newFileName
	metaData.NewLocation = newLocation
	metaData.Md5CheckSum = md5Checksum
	metaData.RemoteTxnID = remoteTxnID
	metaData.DataStackTxnID = dataStackTxnID
	metaData.ErrorMessage = errorMessage
	metaData.MirrorPath = mirrorPath
	byteData, _ := json.Marshal(metaData)
	return string(byteData)
}
