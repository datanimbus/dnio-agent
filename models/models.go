package models

import (
	"time"
)

//LoginAPIRequest - agent login api request structure
type LoginAPIRequest struct {
	AgentID      string `json:"agentId"`
	Password     string `json:"password"`
	AgentVersion string `json:"agentVersion"`
}

//LoginAPIResponse - agent login api response structure
type LoginAPIResponse struct {
	Message      string `json:"message"`
	AgentData    string `json:"agentData" bson:"agentData"`
	Token        string `json:"token"`
	RefreshToken string `json:"rToken"`
	ExpiresIn    string `json:"expiresIn"`
}

//AgentDataFromIM - agent information from IM
type AgentData struct {
	ID                  string `json:"_id"`
	Active              bool   `json:"active"`
	AppName             string `json:"app"`
	AgentName           string `json:"name"`
	AgentVersion        int64  `json:"__v"`
	EncryptFile         bool   `json:"encryptFile"`
	RetainFileOnSuccess bool   `json:"retainFileOnSuccess"`
	RetainFileOnError   bool   `json:"retainFileOnError"`
	Internal            bool   `json:"internal"`
	Token               string `json:"token"`
	Secret              string `json:"secret"`
}

//CentralHeartBeatRequest - agent central heartbeat request structure
type CentralHeartBeatRequest struct {
	MonitoringLedgerEntries []MonitoringLedgerEntry `json:"monitoringLedgerEntries"`
	TransferLedgerEntries   []TransferLedgerEntry   `json:"transferLedgerEntries"`
}

//CentralHeartBeatResponse - response structure to central heartbeat request
type CentralHeartBeatResponse struct {
	TransferLedgerEntries     []TransferLedgerEntry `json:"transferLedgerEntries"`
	Status                    string                `json:"status"`
	AgentMaxConcurrentUploads string                `json:"agentMaxConcurrentUploads"`
}

//TransferLedgerEntry - TransferLedgerEntry structure of DB
type TransferLedgerEntry struct {
	ID             string    `storm:"id" bson:"id"`
	AgentID        string    `json:"agentID" bson:"AgentID"`
	AppName        string    `json:"appName" bson:"AppName"`
	AgentName      string    `json:"agentName" bson:"AgentName"`
	FlowName       string    `json:"flowName" bson:"FlowName"`
	FlowID         string    `json:"flowID" bson:"FlowID"`
	DeploymentName string    `json:"deploymentName" bson:"DeploymentName"`
	Action         string    `json:"action" bson:"Action"`
	MetaData       string    `json:"metaData" bson:"MetaData"`
	Timestamp      time.Time `json:"timestamp" bson:"Timestamp"`
	EntryType      string    `json:"entryType" bson:"EntryType"`
	SentOrRead     bool      `json:"sentOrRead" bson:"SentOrRead"`
	Status         string    `json:"status" bson:"Status"`
}

//MonitoringLedgerEntry - entry structure for flowledger entry
type MonitoringLedgerEntry struct {
	AgentID            string         `storm:"id" json:"agentID" bson:"AgentID"`
	AppName            string         `json:"appName" bson:"AppName"`
	AgentName          string         `json:"agentName" bson:"AgentName"`
	HeartBeatFrequency string         `json:"heartBeatFrequency" bson:"HeartBeatFrequency"`
	MACAddress         string         `json:"macAddress" bson:"MACAddress"`
	IPAddress          string         `json:"ipAddress" bson:"IPAddress"`
	Status             string         `json:"status" bson:"Status"`
	Timestamp          time.Time      `json:"timestamp" bson:"Timestamp"`
	AbsolutePath       string         `json:"absolutePath" bson:"AbsolutePath"`
	PendingFilesCount  []PendingFiles `json:"pendingFiles" bson:"PendingFiles"`
	Release            string         `json:"release" bson:"Release"`
}

//PendingFiles - pending files struct
type PendingFiles struct {
	FlowID string `json:"flowID" bson:"FlowID"`
	Count  int    `json:"count" bson:"Count"`
}

//FlowDefinitionResponse - response structure for flow creation request
type FlowDefinitionResponse struct {
	FlowName                       string                         `json:"flowName"`
	FlowID                         string                         `json:"flowID"`
	FileSuffix                     string                         `json:"fileSuffix"`
	DeploymentName                 string                         `json:"deploymentName"`
	InputDirectory                 []InputDirectoryInfo           `json:"inputDirectories"`
	OutputDirectory                []OutputDirectoryInfo          `json:"outputDirectories"`
	BlockName                      string                         `json:"blockName"`
	StructureID                    string                         `json:"structureID"`
	SequenceNo                     int                            `json:"sequenceNo"`
	Direction                      string                         `json:"direction"`
	UniqueRemoteTransaction        bool                           `json:"uniqueRemoteTransaction"`
	UniqueRemoteTransactionOptions UniqueRemoteTransactionOptions `json:"uniqueRemoteTransactionOptions"`
	FileMaxSize                    int                            `json:"fileMaxSize"`
	FileExtensions                 []FileExtensionStruct          `json:"fileExtensions"`
	FileNameRegexs                 []string                       `json:"fileNameRegexs"`
	Mirror                         bool                           `json:"mirrorInputDirectories"`
	TargetAgentID                  string                         `json:"targetAgentID"`
	Timer                          TimeBoundProperties            `json:"timer"`
	ErrorBlocks                    bool                           `json:"errorBlocks"`
}

//InputDirectoryInfo - json data for input directory information
type InputDirectoryInfo struct {
	Path                string `json:"path"`
	WatchSubDirectories bool   `json:"watchSubDirectories"`
	RootPath            string `json:"rootPath"`
}

//OutputDirectoryInfo - json data for output directory information
type OutputDirectoryInfo struct {
	Path string `json:"path"`
}

//FileExtensionStruct - fileExtension struct
type FileExtensionStruct struct {
	Extension string `json:"extension"`
	Custom    bool   `json:"custom"`
}

//UniqueRemoteTransactionOptions - options for File-X for unique transaction ...
type UniqueRemoteTransactionOptions struct {
	FileName bool `json:"fileName"`
	Checksum bool `json:"checksum"`
}

//TimeBoundProperties - properties for greenzone, for picking up the file
type TimeBoundProperties struct {
	CronRegEx            string            `json:"cronRegEx"`
	Enabled              bool              `json:"enabled"`
	RestrictedZoneAction string            `json:"restrictedZoneAction"`
	TimeBounds           []TimeBoundStruct `json:"timebounds"`
}

//TimeBoundStruct for range of weeks, months
type TimeBoundStruct struct {
	From string `json:"from"`
	To   string `json:"to"`
}

//EncryptionDecryptionTool - encryption decryption tool
type EncryptionDecryptionTool struct {
	Password       string `json:"password"`
	InputFilePath  string `json:"inputFilePath"`
	OutputFilePath string `json:"outputFilePath"`
}

//EncryptionDecryptionToolMessage - encryption decryption too message
type EncryptionDecryptionToolMessage struct {
	Message string `json:"message"`
}
