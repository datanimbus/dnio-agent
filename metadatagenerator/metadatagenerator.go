package metadatagenerator

import (
	"encoding/json"
)

//ErrorMessageMetaData - file process error metadata
type ErrorMessageMetaData struct {
	ErrorMessage string `json:"errorMessage"`
}

//GenerateErrorMessageMetaData - generate error message metadata
func GenerateErrorMessageMetaData(message string) string {
	metaData := ErrorMessageMetaData{}
	metaData.ErrorMessage = message
	byteData, _ := json.Marshal(metaData)
	return string(byteData)
}
