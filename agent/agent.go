package agent

import (
	"ds-agent/ledgers"
	"ds-agent/messagegenerator"
	"ds-agent/metadatagenerator"
	"ds-agent/models"
	"ds-agent/utils"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/appveen/go-log/logger"
	"github.com/gorilla/mux"
)

const (
	//AlreadyRunningAgent - agent is already running or not
	AlreadyRunningAgent = "ALREADY_RUNNING"

	//DisabledAgent - agent is disabled
	DisabledAgent = "DISABLED"

	//ReleaseChanged - release changed
	ReleaseChanged = "RELEASE_CHANGED"

	//MaxChunkSize - for encryption and decryption, and it will be same forever we cannot change it otherwise there will be consequences.
	MaxChunkSize = 1024 * 1024 * 100

	//BytesToSkipWhileDecrypting - redundant bytes not actual file data
	BytesToSkipWhileDecrypting = 64

	//DBSizeUpdateTimeRegEx - time when agen will be in pause state
	DBSizeUpdateTimeRegEx = "@midnight"
)

//AgentDetails - details of agent
type AgentDetails struct {
	UUID                   string
	AgentID                string
	AgentName              string
	Password               string
	AgentVersion           int64
	AppName                string
	UploadRetryCounter     string
	DownloadRetryCounter   string
	BaseURL                string
	HeartBeatFrequency     string
	AgentType              string
	LogLevel               string
	SentinelPortNumber     string
	SentinelMaxMissesCount string
	EncryptFile            bool
	RetainFileOnSuccess    bool
	RetainFileOnError      bool
	AgentPortNumber        string
	Release                string
	MaxLogBackUpIndex      string
	LogMaxFileSize         string
	LogRotationType        string
	ExecutableDirectory    string
	CentralFolder          string
	ConfFilePath           string
	AbsolutePath           string
	AppFolderPath          string
	TransferLedgerPath     string
	MonitoringLedgerPath   string
	Flows                  map[string]map[string]map[string]string
	FlowsChannel           map[string]map[string]chan string
	Utils                  utils.UtilityService
	TransferLedger         ledgers.TransferLedgerService
	MonitoringLedger       ledgers.MonitorLedgerService
	MaxConcurrentDownloads int
	MaxConcurrentUploads   int
	Server                 *http.Server
	Logger                 logger.Logger
	InteractiveMode        bool
	MACAddress             string
	IPAddress              string
	FilesUploading         int64
	FilesDownloading       int64
	Token                  string
}

//AgentService - task of agent
type AgentService interface {
	StartAgent() error
	getRunningOrPendingFlowsFromB2BManager()
	initAgentServer() error
	initCentralHeartBeat(wg *sync.WaitGroup)
	addEntryToTransferLedger(flowName string, flowID string, deploymentName string, action string, metaData string, time time.Time, entryType string, sentOrRead bool) error
	createFlowFolderStructure(flowFolder string)
	FetchHTTPClient() *http.Client
}

//SetUpAgent -  setting up the datastack-agent
func SetUpAgent(centralFolder string, DATASTACKAgent *AgentDetails, pass string, interactive bool, logger logger.Logger) error {
	var err error
	DATASTACKAgent.FilesUploading = 0
	DATASTACKAgent.FilesDownloading = 0
	DATASTACKAgent.Password = pass
	DATASTACKAgent.Utils = &utils.UtilsService{}
	DATASTACKAgent.InteractiveMode = interactive
	DATASTACKAgent.ExecutableDirectory, _, _ = utils.GetExecutablePathAndName()
	DATASTACKAgent.ConfFilePath = filepath.Join(DATASTACKAgent.ExecutableDirectory, "..", "conf", "agent.conf")
	DATASTACKAgent.CentralFolder = centralFolder
	DATASTACKAgent.AbsolutePath, _ = os.Getwd()
	if DATASTACKAgent.BaseURL != "" && !strings.Contains(DATASTACKAgent.BaseURL, "https") {
		DATASTACKAgent.BaseURL = "https://" + DATASTACKAgent.BaseURL
	}

	headers := map[string]string{}
	headers["DATA-STACK-App-Name"] = DATASTACKAgent.AppName
	headers["DATA-STACK-Agent-Id"] = DATASTACKAgent.AgentID
	headers["DATA-STACK-Agent-Name"] = DATASTACKAgent.AgentName
	headers["DATA-STACK-Mac-Address"] = DATASTACKAgent.MACAddress
	headers["DATA-STACK-Ip-Address"] = DATASTACKAgent.IPAddress
	headers["DATA-STACK-Agent-Type"] = DATASTACKAgent.AgentType
	DATASTACKAgent.Logger = logger
	DATASTACKAgent.IPAddress = DATASTACKAgent.Utils.GetLocalIP()
	list, err := DATASTACKAgent.Utils.GetMacAddr()
	if err != nil {
		DATASTACKAgent.Logger.Error(fmt.Sprintf("Mac Address fetching error -: %s", err))
		os.Exit(0)
	}
	DATASTACKAgent.MACAddress = list[0]

	DATASTACKAgent.AppFolderPath = DATASTACKAgent.ExecutableDirectory + string(os.PathSeparator) + ".." + string(os.PathSeparator) + string(os.PathSeparator) + "data" + string(os.PathSeparator) + strings.Replace(DATASTACKAgent.AppName, " ", "_", -1)
	DATASTACKAgent.TransferLedgerPath = DATASTACKAgent.ExecutableDirectory + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "conf" + string(os.PathSeparator) + "transfer-ledger.db"
	DATASTACKAgent.MonitoringLedgerPath = DATASTACKAgent.ExecutableDirectory + string(os.PathSeparator) + ".." + string(os.PathSeparator) + string(os.PathSeparator) + "conf" + string(os.PathSeparator) + "monitoring-ledger.db"
	DATASTACKAgent.Utils.CheckFolderExistsOrCreateFolder(DATASTACKAgent.AppFolderPath)
	DATASTACKAgent.TransferLedger, err = ledgers.InitTransferLedger(DATASTACKAgent.TransferLedgerPath, "")
	if err != nil {
		DATASTACKAgent.Logger.Error(messagegenerator.ExtractErrorMessageFromErrorObject(err))
		os.Exit(0)
	}
	DATASTACKAgent.MonitoringLedger, err = ledgers.InitMonitoringLedger(DATASTACKAgent.MonitoringLedgerPath, "")
	if err != nil {
		DATASTACKAgent.Logger.Error(messagegenerator.ExtractErrorMessageFromErrorObject(err))
		os.Exit(0)
	}

	DATASTACKAgent.Flows = make(map[string]map[string]map[string]string)
	DATASTACKAgent.FlowsChannel = make(map[string]map[string]chan string)
	return nil
}

//StartAgent - Entry point function to invoke remote machine agent for file upload/download
func (DATASTACKAgent *AgentDetails) StartAgent() error {
	var wg sync.WaitGroup
	DATASTACKAgent.Logger.Info("Agent Starting....")
	DATASTACKAgent.Logger.Info("Agent ID : %s", DATASTACKAgent.AgentID)
	DATASTACKAgent.Logger.Info("Agent Name : %s", DATASTACKAgent.AgentName)
	DATASTACKAgent.Logger.Info("Agent Version : %v", DATASTACKAgent.AgentVersion)
	DATASTACKAgent.Logger.Info("App Name : %s", DATASTACKAgent.AppName)
	DATASTACKAgent.Logger.Debug("Upload Retry Counter : %s", DATASTACKAgent.UploadRetryCounter)
	DATASTACKAgent.Logger.Debug("Download Retry Counter : %s", DATASTACKAgent.DownloadRetryCounter)
	DATASTACKAgent.Logger.Debug("Log Level : %s", DATASTACKAgent.LogLevel)
	DATASTACKAgent.Logger.Debug("Encrypt File : %t", DATASTACKAgent.EncryptFile)
	DATASTACKAgent.Logger.Debug("Retain File On Success : %t", DATASTACKAgent.RetainFileOnSuccess)
	DATASTACKAgent.Logger.Debug("Retain File On Error : %t", DATASTACKAgent.RetainFileOnError)
	DATASTACKAgent.Logger.Debug("Max Concurrent Uploads : %v", DATASTACKAgent.MaxConcurrentUploads)
	DATASTACKAgent.Logger.Debug("Max Concurrent Downloads : %v", DATASTACKAgent.MaxConcurrentDownloads)
	DATASTACKAgent.Logger.Debug("Absolute File Path : %s", DATASTACKAgent.AbsolutePath)
	DATASTACKAgent.Logger.Debug("Conf File Path : %s", DATASTACKAgent.ConfFilePath)
	DATASTACKAgent.Logger.Info("Encryption Type : Buffered Encryption")
	DATASTACKAgent.Logger.Info("Interactive mode : %v", DATASTACKAgent.InteractiveMode)

	err := DATASTACKAgent.MonitoringLedger.AddOrUpdateEntry(&models.MonitoringLedgerEntry{
		AgentID:            DATASTACKAgent.AgentID,
		AgentName:          DATASTACKAgent.AgentName,
		AppName:            DATASTACKAgent.AppName,
		MACAddress:         DATASTACKAgent.MACAddress,
		IPAddress:          DATASTACKAgent.IPAddress,
		HeartBeatFrequency: DATASTACKAgent.HeartBeatFrequency,
		Status:             ledgers.RUNNING,
		Timestamp:          time.Now(),
		AbsolutePath:       DATASTACKAgent.AbsolutePath,
	})
	if err != nil {
		DATASTACKAgent.Logger.Error(fmt.Sprintf("%s", err))
		os.Exit(0)
	}
	DATASTACKAgent.getRunningOrPendingFlowsFromB2BManager()
	DATASTACKAgent.Logger.Info("Maximum concurrent watcher limited to 1024.")
	DATASTACKAgent.Logger.Info("Starting central Heartbeat")
	go DATASTACKAgent.initCentralHeartBeat(&wg)
	DATASTACKAgent.Logger.Debug("Starting Agent Server ...")
	go DATASTACKAgent.initAgentServer()
	wg.Wait()
	return nil
}

//GetRunningORPendingFlowFromPartnerManagerAfterRestart - get details of running or pending flows associated with this agent
func (DATASTACKAgent *AgentDetails) getRunningOrPendingFlowsFromB2BManager() {
	DATASTACKAgent.Logger.Info("Fetching the flow(s) information from BM")
	var client = DATASTACKAgent.FetchHTTPClient()

	data := models.CentralHeartBeatResponse{}
	headers := make(map[string]string)
	headers["Authorization"] = "JWT " + DATASTACKAgent.Token
	URL := DATASTACKAgent.BaseURL + "/api/a/bm/{app}/agent/utils/{agentId}/init"
	URL = strings.Replace(URL, "{app}", DATASTACKAgent.AppName, -1)
	URL = strings.Replace(URL, "{agentId}", DATASTACKAgent.AgentID, -1)
	DATASTACKAgent.Logger.Info("Making Request at -: %s", URL)
	err := DATASTACKAgent.Utils.MakeJSONRequest(client, URL, nil, headers, &data)
	if err != nil {
		DATASTACKAgent.addEntryToTransferLedger("", "", "", ledgers.AGENTSTARTERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		if strings.Contains(messagegenerator.ExtractErrorMessageFromErrorObject(err), messagegenerator.InvalidIP) {
			DATASTACKAgent.Logger.Error("IP is not trusted, stopping the agent")
		} else {
			DATASTACKAgent.Logger.Error(fmt.Sprintf("%s", err))
		}
		os.Exit(0)
	}

	switch data.Status {
	case AlreadyRunningAgent:
		DATASTACKAgent.Logger.Error(messagegenerator.AgentAlreadyRunningError)
		os.Exit(0)
	case DisabledAgent:
		DATASTACKAgent.Logger.Error(messagegenerator.AgentDisabledError)
		os.Exit(0)
	case ReleaseChanged:
		DATASTACKAgent.Logger.Error(messagegenerator.ReleaseVersionChangedError)
		os.Exit(0)
	}

	DATASTACKAgent.Logger.Info("%v flows fetched ", len(data.TransferLedgerEntries))
	for _, entry := range data.TransferLedgerEntries {
		DATASTACKAgent.Logger.Info("Action - ", entry.Action)
	}
}

//InitCentralHeartBeat - heartbeat for agents
func (DATASTACKAgent *AgentDetails) initCentralHeartBeat(wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	var client = DATASTACKAgent.FetchHTTPClient()
	frequency, err := strconv.Atoi(DATASTACKAgent.HeartBeatFrequency)
	if err != nil {
		frequency = 10
	}
	for {
		monitoringLedgerEntries, err := DATASTACKAgent.MonitoringLedger.GetAllEntries()
		if err != nil {
			DATASTACKAgent.addEntryToTransferLedger("", "", "", ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
			continue
		}
		transferLedgerEntries, err := DATASTACKAgent.TransferLedger.GetUnsentNotifications()
		if err != nil {
			DATASTACKAgent.addEntryToTransferLedger("", "", "", ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
			continue
		}
		request := models.CentralHeartBeatRequest{
			MonitoringLedgerEntries: monitoringLedgerEntries,
			TransferLedgerEntries:   transferLedgerEntries,
		}

		request.MonitoringLedgerEntries[0].Release = DATASTACKAgent.Release
		data := models.CentralHeartBeatResponse{}
		headers := make(map[string]string)
		headers["Authorization"] = "JWT " + DATASTACKAgent.Token
		headers["HeartBeatFrequency"] = DATASTACKAgent.HeartBeatFrequency
		DATASTACKAgent.Logger.Trace("Heartbeat Headers %s", headers["HeartBeatFrequency"])
		URL := DATASTACKAgent.BaseURL + "/api/a/bm/{app}/agent/utils/{agentId}/heartbeat"
		URL = strings.Replace(URL, "{app}", DATASTACKAgent.AppName, -1)
		URL = strings.Replace(URL, "{agentId}", DATASTACKAgent.AgentID, -1)
		DATASTACKAgent.Logger.Info("Making Request at -: %s", URL)
		err = DATASTACKAgent.Utils.MakeJSONRequest(client, URL, request, headers, &data)
		if err != nil {
			DATASTACKAgent.addEntryToTransferLedger("", "", "", ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
			DATASTACKAgent.Logger.Error("Heartbeat Error %s", messagegenerator.ExtractErrorMessageFromErrorObject(err))
			transferLedgerEntries = nil
			monitoringLedgerEntries = nil
			time.Sleep(time.Duration(frequency) * time.Second)
			continue
		}

		if data.AgentMaxConcurrentUploads != strconv.Itoa(DATASTACKAgent.MaxConcurrentUploads) {
			DATASTACKAgent.Logger.Info("Updating agent max concurrent uploads to %s", data.AgentMaxConcurrentUploads)
			DATASTACKAgent.MaxConcurrentUploads, err = strconv.Atoi(data.AgentMaxConcurrentUploads)
			if err != nil {
				DATASTACKAgent.Logger.Error("error in updating max concurrent uploads for agent ", err)
				DATASTACKAgent.MaxConcurrentUploads = 5
			}
		}

		for _, entry := range transferLedgerEntries {
			err = DATASTACKAgent.TransferLedger.UpdateSentOrReadFieldOfEntry(&entry, true)
			if err != nil {
				DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
				continue
			}
		}

		transferLedgerEntries = nil
		monitoringLedgerEntries = nil
		time.Sleep(time.Duration(frequency) * time.Second)
	}
}

//InitAgentServer start an server from agent side
func (DATASTACKAgent *AgentDetails) initAgentServer() error {
	r := mux.NewRouter()
	http.Handle("/", r)

	if DATASTACKAgent.AgentPortNumber == "" {
		DATASTACKAgent.Server = &http.Server{
			Addr: ":63859",
		}
	} else {
		DATASTACKAgent.Server = &http.Server{
			Addr: ":" + DATASTACKAgent.AgentPortNumber,
		}
	}
	DATASTACKAgent.Logger.Info("Agent Server Started")
	err := DATASTACKAgent.Utils.StartHTTPServer(DATASTACKAgent.Server)
	if err != nil {
		DATASTACKAgent.Logger.Error("Agent already Running Agent On Same Machine ", err)
		os.Exit(0)
	}
	return err
}

//FetchHTTPClient -  getting http client with all certs
func (DATASTACKAgent *AgentDetails) FetchHTTPClient() *http.Client {
	client := DATASTACKAgent.Utils.GetNewHTTPClient(nil)
	return client
}

func (DATASTACKAgent *AgentDetails) addEntryToTransferLedger(flowName string, flowID string, deploymentName string, action string, metaData string, time time.Time, entryType string, sentOrRead bool) error {
	return DATASTACKAgent.TransferLedger.AddEntry(&models.TransferLedgerEntry{
		AgentID:        DATASTACKAgent.AgentID,
		AgentName:      DATASTACKAgent.AgentName,
		AppName:        DATASTACKAgent.AppName,
		FlowName:       flowName,
		FlowID:         flowID,
		DeploymentName: deploymentName,
		Action:         action,
		MetaData:       metaData,
		Timestamp:      time,
		EntryType:      entryType,
		SentOrRead:     sentOrRead,
	})
}

//CreateFlowFolderStructure - create flow folder structure
func (DATASTACKAgent *AgentDetails) createFlowFolderStructure(flowFolder string) error {
	DATASTACKAgent.Utils.CheckFolderExistsOrCreateFolder(flowFolder)
	DATASTACKAgent.Utils.CheckFolderExistsOrCreateFolder(flowFolder + string(os.PathSeparator) + "input")
	DATASTACKAgent.Utils.CheckFolderExistsOrCreateFolder(flowFolder + string(os.PathSeparator) + "processing")
	DATASTACKAgent.Utils.CheckFolderExistsOrCreateFolder(flowFolder + string(os.PathSeparator) + "output")
	DATASTACKAgent.Utils.CheckFolderExistsOrCreateFolder(flowFolder + string(os.PathSeparator) + "error")
	DATASTACKAgent.Utils.CheckFolderExistsOrCreateFolder(flowFolder + string(os.PathSeparator) + "done")
	return nil
}
