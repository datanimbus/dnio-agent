package messagegenerator

import (
	"strconv"
)

// InvalidIP - Trsuted IP error message
var InvalidIP = "Error - Invalid IP"

// AgentAlreadyRunningError - agent is already running error
var AgentAlreadyRunningError = "The agent is already running on same/different machine"

// AgentDisabledError - agent is disabled from DATASTACK error
var AgentDisabledError = "The agent has been stopped/disabled at the server"

// VaultVersionChangedError - old agent whose password was changed
var VaultVersionChangedError = "The agent version has been changed, please download the new agent"

// ReleaseVersionChangedError - release version has been changed
var ReleaseVersionChangedError = "The release version has been changed, please download the new agent"

// FlowAlreadyRunningError -  flow running error
var FlowAlreadyRunningError = "Flow Already Exist and Running."

// NoFlowsExistError - no flows exists error
var NoFlowsExistError = "No such Flow Exist."

// EncryptDecryptFileHandlerError - error during encryption or decryption
var EncryptDecryptFileHandlerError = "Input filepath and Output filepath cannot be same, please change the output filepath and try again"

// ExtractErrorMessageFromErrorObject - extract error message from error object
func ExtractErrorMessageFromErrorObject(err error) string {
	return err.Error()
}

// GetErrorMessageForLargeFileName - return error message for large file name
func GetErrorMessageForLargeFileName(fileName string) string {
	return "The Input file " + fileName + " could not be processed. The file name has more than 100 characters. Please rename the file" + fileName + " to have less that 100 characters in its name and try again."
}

// GetErrorMessageForLargeFileSize - return error message for large file size
func GetErrorMessageForLargeFileSize(fileName string, fileSize int64, maxFileSize int64) string {
	return "The Input file " + fileName + " could not be processed.The filesize is more than the permissible filesize of " + strconv.Itoa(int(maxFileSize)) + " bytes configured at DATASTACK. If possible, reduce the file size by splitting them into two or more files and placing them in the input folder of the flow."
}
