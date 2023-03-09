package models

import (
	"time"
)

// LoginAPIRequest - agent login api request structure
type LoginAPIRequest struct {
	AgentID      string `json:"agentId"`
	Password     string `json:"password"`
	AgentVersion string `json:"agentVersion"`
	IPAddress    string `json:"ipAddress"`
	MACAddress   string `json:"macAddress"`
}

// LoginAPIResponse - agent login api response structure
type LoginAPIResponse struct {
	Message string `json:"message"`
}

// AgentDataFromIM - agent information from IM
type AgentData struct {
	ID                     string `json:"_id"`
	Active                 bool   `json:"active"`
	AppName                string `json:"app"`
	AgentName              string `json:"name"`
	AgentVersion           int64  `json:"__v"`
	EncryptFile            bool   `json:"encryptFile"`
	RetainFileOnSuccess    bool   `json:"retainFileOnSuccess"`
	RetainFileOnError      bool   `json:"retainFileOnError"`
	Internal               bool   `json:"internal"`
	Token                  string `json:"token"`
	Secret                 string `json:"secret"`
	EncryptionKey          string `json:"encryptionKey"`
	UploadRetryCounter     string `json:"uploadRetryCounter"`
	DownloadRetryCounter   string `json:"downloadRetryCounter"`
	MaxConcurrentUploads   int    `json:"maxConcurrentUploads"`
	MaxConcurrentDownloads int    `json:"maxConcurrentDownloads"`
}

// CentralHeartBeatRequest - agent central heartbeat request structure
type CentralHeartBeatRequest struct {
	MonitoringLedgerEntries []MonitoringLedgerEntry `json:"monitoringLedgerEntries"`
	TransferLedgerEntries   []TransferLedgerEntry   `json:"transferLedgerEntries"`
}

// CentralHeartBeatResponse - response structure to central heartbeat request
type CentralHeartBeatResponse struct {
	TransferLedgerEntries     []TransferLedgerEntry `json:"transferLedgerEntries"`
	Status                    string                `json:"status"`
	AgentMaxConcurrentUploads int                   `json:"agentMaxConcurrentUploads"`
}

// TransferLedgerEntry - TransferLedgerEntry structure of DB
type TransferLedgerEntry struct {
	ID             string    `storm:"id" bson:"id"`
	AgentID        string    `json:"agentId" bson:"AgentID"`
	AppName        string    `json:"appName" bson:"AppName"`
	AgentName      string    `json:"agentName" bson:"AgentName"`
	FlowName       string    `json:"flowName" bson:"FlowName"`
	FlowID         string    `json:"flowId" bson:"FlowID"`
	DeploymentName string    `json:"deploymentName" bson:"DeploymentName"`
	Action         string    `json:"action" bson:"Action"`
	MetaData       string    `json:"metaData" bson:"MetaData"`
	Timestamp      time.Time `json:"timestamp" bson:"Timestamp"`
	EntryType      string    `json:"entryType" bson:"EntryType"`
	SentOrRead     bool      `json:"sentOrRead" bson:"SentOrRead"`
	Status         string    `json:"status" bson:"Status"`
}

// MonitoringLedgerEntry - entry structure for flowledger entry
type MonitoringLedgerEntry struct {
	AgentID            string         `storm:"id" json:"agentId" bson:"AgentID"`
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

// PendingFiles - pending files struct
type PendingFiles struct {
	FlowID string `json:"flowId" bson:"FlowID"`
	Count  int    `json:"count" bson:"Count"`
}

// FlowDefinitionResponse - response structure for flow creation request
type FlowDefinitionResponse struct {
	FlowName                       string                         `json:"flowName"`
	FlowID                         string                         `json:"flowId"`
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

// InputDirectoryInfo - json data for input directory information
type InputDirectoryInfo struct {
	Path                string `json:"path"`
	WatchSubDirectories bool   `json:"watchSubDirectories"`
	RootPath            string `json:"rootPath"`
}

// OutputDirectoryInfo - json data for output directory information
type OutputDirectoryInfo struct {
	Path string `json:"path"`
}

// FileExtensionStruct - fileExtension struct
type FileExtensionStruct struct {
	Extension string `json:"extension"`
	Custom    bool   `json:"custom"`
}

// UniqueRemoteTransactionOptions - options for File-X for unique transaction ...
type UniqueRemoteTransactionOptions struct {
	FileName bool `json:"fileName"`
	Checksum bool `json:"checksum"`
}

// TimeBoundProperties - properties for greenzone, for picking up the file
type TimeBoundProperties struct {
	CronRegEx            string            `json:"cronRegEx"`
	Enabled              bool              `json:"enabled"`
	RestrictedZoneAction string            `json:"restrictedZoneAction"`
	TimeBounds           []TimeBoundStruct `json:"timebounds"`
}

// TimeBoundStruct for range of weeks, months
type TimeBoundStruct struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// EncryptionDecryptionTool - encryption decryption tool
type EncryptionDecryptionTool struct {
	Password       string `json:"password"`
	InputFilePath  string `json:"inputFilePath"`
	OutputFilePath string `json:"outputFilePath"`
}

// EncryptionDecryptionToolMessage - encryption decryption too message
type EncryptionDecryptionToolMessage struct {
	Message string `json:"message"`
}

// FlowWatcherProperties - watcher properties of that flow
type FlowWatcherProperties struct {
	AppName                string                         `json:"appName"`
	AgentName              string                         `json:"agentName"`
	FlowName               string                         `json:"flowName"`
	FlowID                 string                         `json:"flowId"`
	InputFolder            string                         `json:"inputFolder"`
	BlockName              string                         `json:"blockName"`
	StructureID            string                         `json:"structureID"`
	UniqueRemoteTxn        bool                           `json:"uniqueRemoteTxn"`
	UniqueRemoteTxnOptions UniqueRemoteTransactionOptions `json:"uniqueRemoteTxnOptions"`
	FileExtensions         []FileExtensionStruct          `json:"fileExtensions"`
	FileNameRegexes        []string                       `json:"fileRegex"`
	MirrorEnabled          bool                           `json:"mirrorEnabled"`
	OutputDirectories      []OutputDirectoryInfo          `json:"outputDirectories"`
	TargetAgentID          string                         `json:"targetAgentID"`
	WatcherType            string                         `json:"watcherType"`
	Timer                  TimeBoundProperties            `json:"timer"`
	Listener               bool                           `json:"listener"`
	ErrorBlocks            bool                           `json:"errorBlocks"`
}

// MirrorDirectoryMetaData - mirror directory metadata
type MirrorDirectoryMetaData struct {
	OperatingSystem string                `json:"operatingSystem"`
	MirrorPaths     []string              `json:"mirrorPaths"`
	OutputDirectory []OutputDirectoryInfo `json:"outputDirectories"`
	TargetAgentID   string                `json:"targetAgentID"`
}

// FileUploadMetaData - utility structure for file upload metadata
type FileUploadMetaData struct {
	FlowName                string                `json:"flowName"`
	FlowID                  string                `json:"flowId"`
	AgentID                 string                `json:"agentId"`
	AgentName               string                `json:"agentName"`
	AppName                 string                `json:"appName"`
	OriginalFileName        string                `json:"originalFileName"`
	OriginalFilePath        string                `json:"originalFilePath"`
	NewFileName             string                `json:"newFileName"`
	NewLocation             string                `json:"newLocation"`
	Md5CheckSum             string                `json:"md5CheckSum"`
	RemoteTxnID             string                `json:"remoteTxnID"`
	DataStackTxnID          string                `json:"dataStackTxnID"`
	InputDirectory          string                `json:"inputDirectory"`
	BlockName               string                `json:"blockName"`
	StructureID             string                `json:"structureID"`
	ErrorMessage            string                `json:"errorMessage"`
	UniqueRemoteTransaction string                `json:"uniqueRemoteTransaction"`
	UniqueFileName          string                `json:"uniqueFileName"`
	UniqueChecksum          string                `json:"uniqueChecksum"`
	DownloadAgentID         string                `json:"downloadAgentID"`
	MirrorPath              string                `json:"mirrorPath"`
	Token                   string                `json:"DATASTACKFileToken"`
	FlowWatcherProperties   FlowWatcherProperties `json:"flowWatcherProperties"`
}

// ErrorFlowRequestData - request data for error flow
type ErrorFlowRequestData struct {
	AppName    string `json:"appName"`
	AgentID    string `json:"agentId"`
	AgentName  string `json:"agentName"`
	FlowID     string `json:"flowId"`
	FlowName   string `json:"flowName"`
	NodeType   string `json:"nodeType"`
	NodeID     string `json:"nodeId"`
	Message    string `json:"message"`
	StackTrace string `json:"stackTrace"`
	StatusCode string `json:"statusCode"`
}

// PendingFileMetadata - pending file metadata
type QueuedFileMetadata struct {
	FlowProperties FlowWatcherProperties
	TimeStamp      time.Time
	Entry          TransferLedgerEntry
}

// FileUploadErrorMetaData - file upload error meta data
type FileUploadErrorMetaData struct {
	ErrorMessage   string `json:"errorMessage"`
	RemoteTxnID    string `json:"remoteTxnID"`
	DATASTACKTxnID string `json:"dataStackTxnID"`
}

// InteractionMetadata - metadata for interaction
type InteractionMetadata struct {
	FileSuffix        string   `json:"fileSuffix"`
	InputDirectory    string   `json:"inputDirectory"`
	OutputDirectory   []string `json:"outputDirectory"`
	BlockName         string   `json:"blockName"`
	StructureID       string   `json:"structureID"`
	SequenceNo        int      `json:"sequenceNo"`
	RemoteTxnID       string   `json:"remoteTxnID"`
	DataStackTxnID    string   `json:"dataStackTxnID"`
	OriginalFileName  string   `json:"originalFileName"`
	Md5CheckSum       string   `json:"md5CheckSum"`
	Size              string   `json:"size"`
	MACAddress        string   `json:"macAddress"`
	IPAddress         string   `json:"IPAddress"`
	Encrypt           bool     `json:"encrypt"`
	SuccessFlow       bool     `json:"successFlow"`
	BaseInteractionID string   `json:"baseInteractionID" bson:"baseInteractionID"`
	ReattemptCount    int      `json:"reattemptCount" bson:"reattemptCount"`
	AttemptNo         int      `json:"attemptNo" bson:"attemptNo"`
	MirrorPath        string   `json:"mirrorPath" bson:"mirrorPath"`
	OS                string   `json:"os" bson:"os"`
}

// DownloadFileRequestMetaData - utility structure for any file download request
type DownloadFileRequestMetaData struct {
	FileName              string   `json:"fileName"`
	RemoteTxnID           string   `json:"remoteTxnID"`
	DataStackTxnID        string   `json:"dataStackTxnID"`
	Checksum              string   `json:"checksum"`
	FileLocation          []string `json:"outputDirectory"`
	BlockName             string   `json:"blockName"`
	SequenceNo            string   `json:"sequenceNo"`
	StructureID           string   `json:"structureID"`
	HeaderOutputDirectory string   `json:"headerOutputDirectory"`
	MirrorDirectory       string   `json:"mirrorDirectory"`
	FileID                string   `json:"fileID"`
	Password              string   `json:"password"`
	OperatingSystem       string   `json:"operatingSystem"`
	SuccessBlock          string   `json:"successBlock"`
	ChunkChecksumList     string   `json:"chunkChecksumList"`
	TotalChunks           string   `json:"totalChunks"`
	DownloadAgentID       string   `json:"downloadAgentID"`
}

// FileDownloadErrorMetaData - file download error meta data
type FileDownloadErrorMetaData struct {
	ErrorMessage   string `json:"errorMessage"`
	RemoteTxnID    string `json:"remoteTxnID"`
	DATASTACKTxnID string `json:"dataStackTxnID"`
}

// DownloadFileRequest - request structure for download file
type DownloadFileRequest struct {
	AgentName    string `json:"agentName"`
	AgentID      string `json:"agentId"`
	AgentVersion string `json:"agentVersion"`
	FileName     string `json:"fileName"`
	FlowName     string `json:"flowName"`
	FlowID       string `json:"flowId"`
	AppName      string `json:"appName"`
	FileID       string `json:"fileID"`
}

// FilePostProcessSuccessErrorMetaData - file post process error meta data
type FilePostProcessSuccessErrorMetaData struct {
	ErrorMessage   string `json:"errorMessage"`
	RemoteTxnID    string `json:"remoteTxnID"`
	DATASTACKTxnID string `json:"dataStackTxnID"`
}

// FilePostProcessFailureErrorMetaData - file post process error meta data
type FilePostProcessFailureErrorMetaData struct {
	ErrorMessage   string `json:"errorMessage"`
	RemoteTxnID    string `json:"remoteTxnID"`
	DATASTACKTxnID string `json:"dataStackTxnID"`
}
