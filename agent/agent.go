package agent

import (
	"bytes"
	"ds-agent/ledgers"
	"ds-agent/messagegenerator"
	"ds-agent/metadatagenerator"
	"ds-agent/models"
	"ds-agent/utils"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/appveen/go-log/logger"
	"github.com/gorilla/mux"
	"github.com/robfig/cron"
	uuid "github.com/satori/go.uuid"
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

	pollingUploads = "POLLING_UPLOADS"

	timeBoundUploads = "TIME_BOUND_UPLOADS"

	userTriggeredUploads = "USER_TRIGERRED_UPLOADS"
)

type watcherCounter struct {
	mu sync.Mutex
	x  int
}

func (c *watcherCounter) Add(x int) {
	c.mu.Lock()
	c.x += x
	c.mu.Unlock()
}

func (c *watcherCounter) Value() (x int) {
	c.mu.Lock()
	x = c.x
	c.mu.Unlock()
	return
}

var fileInUploadQueue = sync.Map{}
var numberOfInputDirectoriesForFlow = sync.Map{}

//FlowDirectories - list of input directories
var FlowDirectories = make(map[string][]string)
var watchingDirectoryMap = make(map[string]map[string]bool)
var weHaveToWatchSubDirectories = make(map[string]map[string]bool)
var rootFolderHashMap = make(map[string]map[string]string)
var directoryAllocation = make(map[string]string)

//QueuedFiles - To store pending file data
var QueuedFiles = make(map[string]models.QueuedFileMetadata)

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
	WatchersRunning        watcherCounter
	Paused                 bool
	UploadType             string
	EncryptionKey          string
}

//AgentService - task of agent
type AgentService interface {
	StartAgent() error
	getRunningOrPendingFlowsFromB2BManager()
	initAgentServer() error
	initCentralHeartBeat(wg *sync.WaitGroup)
	addEntryToTransferLedger(flowName string, flowID string, action string, metaData string, time time.Time, entryType string, sentOrRead bool) error
	FetchHTTPClient() *http.Client
	handleFlowCreateStartOrUpdateRequest(entry models.TransferLedgerEntry)
	handleFlowStopRequest(entry models.TransferLedgerEntry)
	handleFlowDeleteRequest(entry models.TransferLedgerEntry)
	createFlowFolderStructure(flowFolder string)
	checkInputDirectoryArrayForWatchingSubdirectories(inputDirectories []models.InputDirectoryInfo, flowID string) []string
	traverseThroughTheInputDirectoryStructure(rootPath string, flowID string) []string
	handleGetMirrorFolderStructure(entry models.TransferLedgerEntry)
	generateMirrorDirectoryStructure(inputDirectory string, flowID string) []string
	handleReceiveMirrorFolderStructure(entry models.TransferLedgerEntry)
	createMirrorOutputDirectoryStructure(mirrorOutputFolderPaths []string, outputDirectory string)
	createMirrorFolderStructureInDoneAndErrorFolder(partnerName string, flowName string, mirrorPaths []string)
	watchNewMirrorFolders(partnerName string, flowName string, inputFolder string, flowID string, partnerID string, deploymentName string, namespace string, blockName string, structureID string, uniqueRemoteTxn bool, uniqueRemoteTxnOptions models.UniqueRemoteTransactionOptions, fileExtensions []models.FileExtensionStruct, fileNameRegexes []string, mirror bool, outputDir []models.OutputDirectoryInfo, targetAgentID string)
	determineCountOfInputDirectories(foldersToBeWatched []string, flowName string) int
	handleUploadFileRequest(entry models.TransferLedgerEntry, blockOpener chan bool)
	handleDownloadFileRequest(entry models.TransferLedgerEntry, blockOpener chan bool)
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

	ok, err := DATASTACKAgent.TransferLedger.DeletePendingUploadRequestFromDB()
	if !ok {
		DATASTACKAgent.Logger.Error("Unable to delete pending upload request from agent error -: ", err)
	}

	DATASTACKAgent.Flows = make(map[string]map[string]map[string]string)
	DATASTACKAgent.FlowsChannel = make(map[string]map[string]chan string)
	DATASTACKAgent.Paused = false
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
	DATASTACKAgent.Logger.Info("Starting Agent Server ...")
	go DATASTACKAgent.initAgentServer()
	DATASTACKAgent.Logger.Info("Started Queue Uploads")
	go DATASTACKAgent.processQueuedUploads()
	DATASTACKAgent.Logger.Info("Starting Queue Downloads")
	go DATASTACKAgent.processQueuedDownloads()
	DATASTACKAgent.CompactDBHandler()
	// DATASTACKAgent.QueuedFileUploadWatcher()
	DATASTACKAgent.handleQueuedJobs(&wg)
	DATASTACKAgent.Logger.Info("Started Queued Jobs Handler")

	wg.Wait()
	return nil
}

//GetRunningOrPendingFlowsFromB2BManager - get details of running or pending flows associated with this agent
func (DATASTACKAgent *AgentDetails) getRunningOrPendingFlowsFromB2BManager() {
	DATASTACKAgent.Logger.Info("Fetching the flow(s) information from BM")
	var client = DATASTACKAgent.FetchHTTPClient()

	data := models.CentralHeartBeatResponse{}
	headers := make(map[string]string)
	headers["Authorization"] = "JWT " + DATASTACKAgent.Token
	headers["DATA-STACK-Agent-Paused"] = strconv.FormatBool(DATASTACKAgent.Paused)
	URL := DATASTACKAgent.BaseURL + "/b2b/bm/{app}/agent/utils/{agentId}/init"
	URL = strings.Replace(URL, "{app}", DATASTACKAgent.AppName, -1)
	URL = strings.Replace(URL, "{agentId}", DATASTACKAgent.AgentID, -1)
	DATASTACKAgent.Logger.Info("Making Request at -: %s", URL)
	err := DATASTACKAgent.Utils.MakeJSONRequest(client, URL, nil, headers, &data)
	if err != nil {
		DATASTACKAgent.addEntryToTransferLedger("", "", ledgers.AGENTSTARTERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		if strings.Contains(messagegenerator.ExtractErrorMessageFromErrorObject(err), messagegenerator.InvalidIP) {
			DATASTACKAgent.Logger.Error("IP is not trusted, stopping the agent")
		} else {
			DATASTACKAgent.Logger.Error(fmt.Sprintf("%s", err))
		}
		os.Exit(0)
	}

	DATASTACKAgent.Logger.Info("%v flows fetched ", len(data.TransferLedgerEntries))
	for _, entry := range data.TransferLedgerEntries {
		DATASTACKAgent.Logger.Trace("Action - ", entry.Action)
		switch entry.Action {
		case ledgers.FLOWCREATEREQUEST:
			DATASTACKAgent.handleFlowCreateStartOrUpdateRequest(entry)
			return
		case ledgers.FLOWSTARTREQUEST:
			DATASTACKAgent.handleFlowCreateStartOrUpdateRequest(entry)
			return
		}
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
		if !DATASTACKAgent.Paused {
			monitoringLedgerEntries, err := DATASTACKAgent.MonitoringLedger.GetAllEntries()
			if err != nil {
				DATASTACKAgent.addEntryToTransferLedger("", "", ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
				continue
			}
			transferLedgerEntries, err := DATASTACKAgent.TransferLedger.GetUnsentNotifications()
			if err != nil {
				DATASTACKAgent.addEntryToTransferLedger("", "", ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
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
			URL := DATASTACKAgent.BaseURL + "/b2b/bm/{app}/agent/utils/{agentId}/heartbeat"
			URL = strings.Replace(URL, "{app}", DATASTACKAgent.AppName, -1)
			URL = strings.Replace(URL, "{agentId}", DATASTACKAgent.AgentID, -1)
			DATASTACKAgent.Logger.Info("Making Request at -: %s", URL)
			if !DATASTACKAgent.Paused {
				err = DATASTACKAgent.Utils.MakeJSONRequest(client, URL, request, headers, &data)
			} else {
				continue
			}
			if err != nil {
				DATASTACKAgent.addEntryToTransferLedger("", "", ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
				DATASTACKAgent.Logger.Error("Heartbeat Error %s", messagegenerator.ExtractErrorMessageFromErrorObject(err))
				transferLedgerEntries = nil
				monitoringLedgerEntries = nil
				time.Sleep(time.Duration(frequency) * time.Second)
				continue
			}

			DATASTACKAgent.Logger.Info("Heartbeat Response from Integration Manager - %s", data)
			if !DATASTACKAgent.Paused {
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
			}

			if data.AgentMaxConcurrentUploads != DATASTACKAgent.MaxConcurrentUploads {
				DATASTACKAgent.Logger.Info("Updating AgentMaxConcurrentUploads to %s", data.AgentMaxConcurrentUploads)
				DATASTACKAgent.MaxConcurrentUploads = data.AgentMaxConcurrentUploads
			}

			for _, entry := range data.TransferLedgerEntries {
				if entry.Action != ledgers.CREATEAPIFLOWREQUEST && entry.Action != ledgers.STOPAPIFLOWREQUEST {
					err = DATASTACKAgent.TransferLedger.AddEntry(&entry)
					if err != nil {
						DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
						continue
					}
				}
			}
			for _, entry := range data.TransferLedgerEntries {
				err = DATASTACKAgent.TransferLedger.UpdateSentOrReadFieldOfEntry(&entry, true)
				if err != nil {
					DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
					continue
				}
			}

			transferLedgerEntries = nil
			monitoringLedgerEntries = nil
			time.Sleep(time.Duration(frequency) * time.Second)
		}
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

func (DATASTACKAgent *AgentDetails) addEntryToTransferLedger(flowName string, flowID string, action string, metaData string, time time.Time, entryType string, sentOrRead bool) error {
	return DATASTACKAgent.TransferLedger.AddEntry(&models.TransferLedgerEntry{
		AgentID:    DATASTACKAgent.AgentID,
		AgentName:  DATASTACKAgent.AgentName,
		AppName:    DATASTACKAgent.AppName,
		FlowName:   flowName,
		FlowID:     flowID,
		Action:     action,
		MetaData:   metaData,
		Timestamp:  time,
		EntryType:  entryType,
		SentOrRead: sentOrRead,
	})
}

func (DATASTACKAgent *AgentDetails) handleFlowCreateStartOrUpdateRequest(entry models.TransferLedgerEntry) {
	switch entry.Action {
	case ledgers.FLOWCREATEREQUEST:
		DATASTACKAgent.Logger.Info("Handling Flow Creation Request %s", entry.FlowName)
		DATASTACKAgent.Logger.Debug("Entry details -: %v", entry)
		if DATASTACKAgent.Flows[entry.AppName] != nil && DATASTACKAgent.Flows[entry.AppName][entry.FlowID] != nil {
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWALREADYSTARTED, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.FlowAlreadyRunningError), time.Now(), "OUT", false)
			return
		}
	case ledgers.FLOWSTARTREQUEST:
		DATASTACKAgent.Logger.Info("Handling Flow Start Request %s", entry.FlowName)
		DATASTACKAgent.Logger.Debug("Entry details -: %v", entry)
		if DATASTACKAgent.Flows[entry.AppName] != nil && DATASTACKAgent.Flows[entry.AppName][entry.FlowID] != nil {
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWALREADYSTARTED, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.FlowAlreadyRunningError), time.Now(), "OUT", false)
			return
		}
	case ledgers.FLOWUPDATEREQUEST:
		DATASTACKAgent.Logger.Debug("Handling Flow Update Request - %s", entry.FlowName)
		DATASTACKAgent.Logger.Debug("Entry details -: %v", entry)
		DATASTACKAgent.handleFlowStopRequest(entry)
	}

	flowFolder := DATASTACKAgent.AppFolderPath + string(os.PathSeparator) + strings.Replace(entry.FlowName, " ", "_", -1)
	DATASTACKAgent.createFlowFolderStructure(flowFolder)

	newFlowReq := models.FlowDefinitionResponse{}
	err := json.Unmarshal([]byte(entry.MetaData), &newFlowReq)
	if err != nil {
		DATASTACKAgent.Logger.Error("Flow metadata unmarshalling error %s", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWCREATIONERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		return
	} else {
		if len(newFlowReq.InputDirectory) == 0 {
			defaultInputFolder := flowFolder + string(os.PathSeparator) + "input"
			newFlowReq.InputDirectory = append(newFlowReq.InputDirectory, models.InputDirectoryInfo{Path: defaultInputFolder, WatchSubDirectories: false})
		}
	}
	newFlowReq.FlowName = entry.FlowName
	newFlowReq.FlowID = entry.FlowID
	for i := 0; i < len(newFlowReq.InputDirectory); i++ {
		newFlowReq.InputDirectory[i].Path = returnAbsolutePath(newFlowReq.InputDirectory[i].Path)
	}
	listener := true
	if watchingDirectoryMap[entry.FlowID] == nil {
		watchingDirectoryMap[entry.FlowID] = make(map[string]bool)
	}

	if rootFolderHashMap[entry.FlowID] == nil {
		rootFolderHashMap[entry.FlowID] = make(map[string]string)
	}

	if weHaveToWatchSubDirectories[entry.FlowID] == nil {
		weHaveToWatchSubDirectories[entry.FlowID] = make(map[string]bool)
	}

	var foldersToBeWatched []string
	if listener {
		foldersToBeWatched = DATASTACKAgent.checkInputDirectoryArrayForWatchingSubdirectories(newFlowReq.InputDirectory, entry.FlowID)
		sort.Slice(foldersToBeWatched, func(i, j int) bool {
			return len(foldersToBeWatched[i]) < len(foldersToBeWatched[j])
		})
		fillRootHashMap(foldersToBeWatched, entry.FlowID)
	}

	err = DATASTACKAgent.Utils.CreateOrUpdateFlowConfFile(flowFolder+string(os.PathSeparator)+"flow.conf", &newFlowReq, true, entry.DeploymentName)
	if err != nil {
		DATASTACKAgent.Logger.Error("Update Flow conf file error %s", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWCONFWRITEERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		return
	}
	values, err := DATASTACKAgent.Utils.ReadFlowConfigurationFile(flowFolder + string(os.PathSeparator) + "flow.conf")
	if err != nil {
		DATASTACKAgent.Logger.Error("Read Flow conf file error %s", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWCONFREADERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		return
	}
	DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWCREATED, entry.MetaData, time.Now(), "OUT", false)

	values["parent-directory"] = flowFolder

	if !listener && newFlowReq.Mirror {
		DATASTACKAgent.Logger.Info("Fetching Directory Structure For Mirroring %s", string(entry.MetaData))
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.GETMIRRORFOLDERSTRUCTURE, entry.MetaData, time.Now(), "OUT", false)
	}

	if listener && newFlowReq.Mirror {
		DATASTACKAgent.Logger.Info("Sending Update Request to receive the Input Directory Structure for Mirroring")
		DATASTACKAgent.handleGetMirrorFolderStructure(entry)
	}

	if DATASTACKAgent.Flows[entry.AppName] == nil {
		DATASTACKAgent.Flows[entry.AppName] = make(map[string]map[string]string)
	}
	if DATASTACKAgent.FlowsChannel[entry.AppName] == nil {
		DATASTACKAgent.FlowsChannel[entry.AppName] = make(map[string]chan string)
	}
	DATASTACKAgent.Flows[entry.AppName][values["flow-id"]] = values

	if listener {
		directoryCount := DATASTACKAgent.determineCountOfInputDirectories(foldersToBeWatched, entry.FlowName)
		numberOfInputDirectoriesForFlow.Store(newFlowReq.FlowID, directoryCount)
		DATASTACKAgent.FlowsChannel[entry.AppName][values["flow-id"]] = make(chan string)
	} else {
		DATASTACKAgent.FlowsChannel[entry.AppName][values["flow-id"]] = nil
	}

	//SetUpFlowProperties Object
	properties := models.FlowWatcherProperties{}
	properties.FlowName = values["flow-name"]
	properties.FlowID = entry.FlowID
	properties.BlockName = newFlowReq.BlockName
	properties.StructureID = newFlowReq.StructureID
	properties.UniqueRemoteTxn = newFlowReq.UniqueRemoteTransaction
	properties.UniqueRemoteTxnOptions = newFlowReq.UniqueRemoteTransactionOptions
	properties.FileExtensions = newFlowReq.FileExtensions
	properties.FileNameRegexes = newFlowReq.FileNameRegexs
	properties.OutputDirectories = newFlowReq.OutputDirectory
	properties.TargetAgentID = newFlowReq.TargetAgentID
	properties.Timer = newFlowReq.Timer
	properties.MirrorEnabled = newFlowReq.Mirror
	properties.Listener = listener
	properties.ErrorBlocks = newFlowReq.ErrorBlocks

	if listener {
		for _, inputDirectory := range foldersToBeWatched {
			if _, err := os.Stat(inputDirectory); !os.IsNotExist(err) {
				properties.InputFolder = inputDirectory
				if DATASTACKAgent.isInputDirectoryAvailable(inputDirectory, entry.FlowName) {
					DATASTACKAgent.Logger.Info("Falling back to long polling.")
					DATASTACKAgent.Logger.Info("Long polling interval set at 60 seconds")
					if DATASTACKAgent.WatchersRunning.Value() < 1024 {
						FlowDirectories[properties.FlowID] = append(FlowDirectories[properties.FlowID], inputDirectory)
						DATASTACKAgent.WatchersRunning.Add(1)
						DATASTACKAgent.Logger.Info("Consumed %v/1024 watchers", DATASTACKAgent.WatchersRunning.Value())
						go DATASTACKAgent.pollInputDirectoryEveryXSeconds(entry, properties)
					} else {
						DATASTACKAgent.Logger.Info("Cannot start watcher already 1024 watchers are running")
					}
					// }
				} else {
					value, ok := numberOfInputDirectoriesForFlow.Load(newFlowReq.FlowID)
					if ok {
						numberOfInputDirectoriesForFlow.Store(newFlowReq.FlowID, value.(int)-1)
					}
				}
			} else {
				DATASTACKAgent.Logger.Error(messagegenerator.ExtractErrorMessageFromErrorObject(err))
				DATASTACKAgent.Logger.Error("Input Directory " + inputDirectory + " doesn't exist please create the directory and restart the agent")
			}
		}
	}

	switch entry.Action {
	case ledgers.FLOWCREATEREQUEST:
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWSTARTED, entry.MetaData, time.Now(), "OUT", false)
		DATASTACKAgent.Logger.Info("%s flow create request completed", entry.FlowName)
	case ledgers.FLOWSTARTREQUEST:
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWSTARTED, entry.MetaData, time.Now(), "OUT", false)
		DATASTACKAgent.Logger.Info("%s flow start request completed", entry.FlowName)
	case ledgers.FLOWUPDATEREQUEST:
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWUPDATED, entry.MetaData, time.Now(), "OUT", false)
		DATASTACKAgent.Logger.Info("%s flow update request completed", entry.FlowName)
	}
}

func (DATASTACKAgent *AgentDetails) handleFlowStopRequest(entry models.TransferLedgerEntry) {
	DATASTACKAgent.Logger.Info("Handling Flow Stop Request %s", entry.FlowName)
	DATASTACKAgent.Logger.Debug("entry details -: %v", entry)
	flowID := entry.FlowID
	if DATASTACKAgent.Flows[entry.AppName] == nil || DATASTACKAgent.Flows[entry.AppName][flowID] == nil {
		DATASTACKAgent.Logger.Error("%s %s", entry.FlowName, messagegenerator.NoFlowsExistError)
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWSTOPERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.NoFlowsExistError), time.Now(), "OUT", false)
		return
	}
	data, err := DATASTACKAgent.Utils.ReadFlowConfigurationFile(DATASTACKAgent.Flows[entry.AppName][flowID]["parent-directory"] + string(os.PathSeparator) + "flow.conf")
	if err != nil {
		DATASTACKAgent.Logger.Error("Read flow conf error %s", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWSTOPERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		return
	}
	err = DATASTACKAgent.Utils.CreateOrUpdateFlowConfFile(DATASTACKAgent.Flows[entry.AppName][flowID]["parent-directory"]+string(os.PathSeparator)+"flow.conf", &models.FlowDefinitionResponse{
		FlowName:   entry.FlowName,
		FileSuffix: data["file-suffix"],
		FlowID:     entry.FlowID,
	}, false, entry.DeploymentName)
	if err != nil {
		DATASTACKAgent.Logger.Error("Update flow conf error %s", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWSTOPERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		return
	}
	if DATASTACKAgent.FlowsChannel[entry.AppName][flowID] != nil {
		count, ok := numberOfInputDirectoriesForFlow.Load(entry.FlowID)
		if ok {
			for i := 0; i < count.(int); i++ {
				DATASTACKAgent.FlowsChannel[entry.AppName][flowID] <- "Closing listener"
			}
			DATASTACKAgent.WatchersRunning.Add(-1 * count.(int))
		}
		numberOfInputDirectoriesForFlow.Delete(entry.FlowID)
	}
	delete(DATASTACKAgent.Flows[entry.AppName], flowID)
	delete(DATASTACKAgent.FlowsChannel[entry.AppName], flowID)
	delete(weHaveToWatchSubDirectories, entry.FlowID)
	delete(rootFolderHashMap, entry.FlowID)
	delete(watchingDirectoryMap, entry.FlowID)
	delete(FlowDirectories, entry.FlowID)
	DATASTACKAgent.Logger.Info("%s Flow stop request completed ", entry.FlowName)
	DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWSTOPPED, entry.MetaData, time.Now(), "OUT", false)
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

func returnAbsolutePath(inputDirectory string) string {
	inputDirectory = strings.Replace(inputDirectory, "\\", "\\\\", -1)
	absoluteInputPath, _ := filepath.Abs(inputDirectory)
	if absoluteInputPath[0] == '\\' && absoluteInputPath[1] != '\\' {
		absoluteInputPath = string(os.PathSeparator) + absoluteInputPath
	}
	return absoluteInputPath
}

func fillRootHashMap(inputDirectory []string, flowID string) {
	for i := 0; i <= len(inputDirectory)-2; i++ {
		if weHaveToWatchSubDirectories[flowID][inputDirectory[i]] {
			for j := i; j <= len(inputDirectory)-1; j++ {
				if i == j {
					value, ok := rootFolderHashMap[flowID][inputDirectory[j]]
					if ok {
						if len(value) > len(inputDirectory[i]) {
							rootFolderHashMap[flowID][inputDirectory[j]] = inputDirectory[i]
						}
					} else {
						rootFolderHashMap[flowID][inputDirectory[j]] = inputDirectory[i]
					}
				} else {
					if strings.Contains(inputDirectory[j], inputDirectory[i]) {
						value, ok := rootFolderHashMap[flowID][inputDirectory[j]]
						if ok {
							if len(value) > len(inputDirectory[i]) {
								rootFolderHashMap[flowID][inputDirectory[j]] = inputDirectory[i]
							}
						} else {
							rootFolderHashMap[flowID][inputDirectory[j]] = inputDirectory[i]
						}
					} else {
						value, ok := rootFolderHashMap[flowID][inputDirectory[j]]
						if ok {
							if len(value) > len(inputDirectory[j]) {
								rootFolderHashMap[flowID][inputDirectory[j]] = inputDirectory[j]
							}
						} else {
							rootFolderHashMap[flowID][inputDirectory[j]] = inputDirectory[j]
						}
					}
				}
			}
		} else {
			value, ok := rootFolderHashMap[flowID][inputDirectory[i]]
			if ok {
				if len(value) > len(inputDirectory[i]) {
					rootFolderHashMap[flowID][inputDirectory[i]] = inputDirectory[i]
				}
			} else {
				rootFolderHashMap[flowID][inputDirectory[i]] = inputDirectory[i]
			}
		}
	}
}

func (DATASTACKAgent *AgentDetails) checkInputDirectoryArrayForWatchingSubdirectories(inputDirectories []models.InputDirectoryInfo, flowID string) []string {
	var inputFolderPaths []string
	var subFolderPaths []string
	for _, inputDirectory := range inputDirectories {
		value, contains := watchingDirectoryMap[flowID][inputDirectory.Path]
		if contains {
			if !value {
				watchingDirectoryMap[flowID][inputDirectory.Path] = true
			}

		} else {
			if inputDirectory.WatchSubDirectories {
				subFolderPaths = DATASTACKAgent.traverseThroughTheInputDirectoryStructure(inputDirectory.Path, flowID)
				inputFolderPaths = append(inputFolderPaths, subFolderPaths...)
			} else {
				inputFolderPaths = append(inputFolderPaths, inputDirectory.Path)
				watchingDirectoryMap[flowID][inputDirectory.Path] = true
			}
		}
	}
	return inputFolderPaths
}

func (DATASTACKAgent *AgentDetails) handleGetMirrorFolderStructure(entry models.TransferLedgerEntry) {
	DATASTACKAgent.Logger.Info("Handling Get Mirror Folder Structure Request - %v", entry)
	var mirrorInputFolderPaths []string
	flowDef := models.FlowDefinitionResponse{}
	err := json.Unmarshal([]byte(entry.MetaData), &flowDef)
	if err != nil {
		DATASTACKAgent.Logger.Error(messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.GETMIRRORFOLDERSTRUCTUREERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		return
	}

	for i := 0; i < len(flowDef.InputDirectory); i++ {
		flowDef.InputDirectory[i].Path = returnAbsolutePath(flowDef.InputDirectory[i].Path)
	}

	sort.Slice(flowDef.InputDirectory, func(i, j int) bool { return len(flowDef.InputDirectory[i].Path) < len(flowDef.InputDirectory[j].Path) })

	for _, inputDirectory := range flowDef.InputDirectory {
		if weHaveToWatchSubDirectories[entry.FlowID][inputDirectory.Path] {
			mirrorPaths := DATASTACKAgent.generateMirrorDirectoryStructure(inputDirectory.Path, entry.FlowID)
			mirrorInputFolderPaths = append(mirrorInputFolderPaths, mirrorPaths...)
		} else {
			rootFolder := filepath.Base(inputDirectory.Path)
			mirrorInputFolderPaths = append(mirrorInputFolderPaths, rootFolder)
		}
	}

	DATASTACKAgent.createMirrorFolderStructureInDoneAndErrorFolder(entry.FlowName, mirrorInputFolderPaths)
	mirrorInputData := models.MirrorDirectoryMetaData{}
	mirrorInputData.OperatingSystem = runtime.GOOS
	mirrorInputData.MirrorPaths = append(mirrorInputData.MirrorPaths, mirrorInputFolderPaths...)
	mirrorInputData.OutputDirectory = flowDef.OutputDirectory
	mirrorInputData.TargetAgentID = flowDef.TargetAgentID
	byteData, _ := json.Marshal(mirrorInputData)
	entry.MetaData = string(byteData)
	DATASTACKAgent.Logger.Info("Sending the Input Directory structure to Mirror At the Output %s", string(entry.MetaData))
	DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.RECEIVEMIRRORFOLDERSTRUCTURE, entry.MetaData, time.Now(), "OUT", false)
}

func (DATASTACKAgent *AgentDetails) generateMirrorDirectoryStructure(inputDirectory string, flowID string) []string {
	var mirrorInputFolderPaths []string
	rootPath := rootFolderHashMap[flowID][inputDirectory]
	rootFolder := filepath.Base(rootPath)
	err := filepath.Walk(inputDirectory, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() && path != rootPath {
			mirrorFolder := strings.Split(path, rootPath+string(os.PathSeparator))
			mirrorInputFolderPaths = append(mirrorInputFolderPaths, rootFolder+string(os.PathSeparator)+mirrorFolder[len(mirrorFolder)-1])
		} else if info.IsDir() && path == rootPath {
			mirrorInputFolderPaths = append(mirrorInputFolderPaths, rootFolder)
		}
		return nil
	})
	if err != nil {
		DATASTACKAgent.Logger.Error(err)
	}
	return mirrorInputFolderPaths
}

func (DATASTACKAgent *AgentDetails) createMirrorFolderStructureInDoneAndErrorFolder(flowName string, mirrorPaths []string) {
	var paths []string
	flowFolder := DATASTACKAgent.AppFolderPath + string(os.PathSeparator) + strings.Replace(flowName, " ", "_", -1)
	paths = append(paths, flowFolder+string(os.PathSeparator)+"done")
	paths = append(paths, flowFolder+string(os.PathSeparator)+"error")
	for _, path := range paths {
		DATASTACKAgent.createMirrorOutputDirectoryStructure(mirrorPaths, path)
	}
}

func (DATASTACKAgent *AgentDetails) createMirrorOutputDirectoryStructure(mirrorOutputFolderPaths []string, outputDirectory string) {
	rootPath := returnAbsolutePath(outputDirectory)
	if _, err := os.Stat(outputDirectory); os.IsNotExist(err) {
		DATASTACKAgent.Logger.Error("Output Directory " + outputDirectory + " doesn't exist please create the directory and restart the agent")
		return
	}
	for _, mirrorOutputPath := range mirrorOutputFolderPaths {
		err := DATASTACKAgent.Utils.CheckFolderExistsOrCreateFolder(rootPath + string(os.PathSeparator) + mirrorOutputPath)
		if err != nil {
			DATASTACKAgent.Logger.Error(messagegenerator.ExtractErrorMessageFromErrorObject(err) + " Give proper permission and restart the agent.")
			return
		}
	}
}

func (DATASTACKAgent *AgentDetails) determineCountOfInputDirectories(foldersToBeWatched []string, flowID string) int {
	var directoryCount int
	directoryCount = 0
	for _, inputDirectory := range foldersToBeWatched {
		if inputDirectory == "" || DATASTACKAgent.isInputDirectoryAvailable(inputDirectory, flowID) {
			directoryCount = directoryCount + 1
		}
	}
	return directoryCount
}

func (DATASTACKAgent *AgentDetails) isInputDirectoryAvailable(inputDir string, flowID string) bool {
	if directoryAllocation[inputDir] != "" && directoryAllocation[inputDir] != flowID {
		DATASTACKAgent.Logger.Error("Input Directory " + inputDir + " is already in use by Flow-: " + directoryAllocation[inputDir])
		return false
	}

	if _, err := os.Stat(inputDir); os.IsNotExist(err) {
		DATASTACKAgent.Logger.Error(err)
		return false
	}
	directoryAllocation[inputDir] = flowID
	return true
}

func (DATASTACKAgent *AgentDetails) pollInputDirectoryEveryXSeconds(entry models.TransferLedgerEntry, properties models.FlowWatcherProperties) {
	c := cron.New()
	c.AddFunc("@every 60s", func() {
		DATASTACKAgent.Logger.Info("Listener started on %s ", properties.InputFolder)
		DATASTACKAgent.initExistingUploadsFromInputFolder(properties.InputFolder, entry, properties.BlockName, properties.StructureID, properties.UniqueRemoteTxn, properties.UniqueRemoteTxnOptions, pollingUploads, properties.ErrorBlocks)
		DATASTACKAgent.Logger.Info("Listener stopped on %s ", properties.InputFolder)
	})
	c.Start()
	end := <-DATASTACKAgent.FlowsChannel[properties.AppName][properties.FlowID]
	c.Stop()
	DATASTACKAgent.Logger.Info("%s on directory %s for agent %s and flow %s", end, properties.InputFolder, properties.AgentName, properties.FlowName)
	delete(directoryAllocation, properties.InputFolder)
}

func (DATASTACKAgent *AgentDetails) initExistingUploadsFromInputFolder(inputFolder string, entry models.TransferLedgerEntry, blockName string, structureID string, uniqueRemoteTxn bool, uniqueRemoteTxnOptions models.UniqueRemoteTransactionOptions, uploadType string, errorBlocks bool) {
	flowFolder := DATASTACKAgent.AppFolderPath + string(os.PathSeparator) + strings.Replace(entry.FlowName, " ", "_", -1)
	values, err := DATASTACKAgent.Utils.ReadFlowConfigurationFile(flowFolder + string(os.PathSeparator) + "flow.conf")
	if err != nil {
		DATASTACKAgent.Logger.Error(fmt.Sprintf("%s", err))
	}
	newFlowReq := models.FlowDefinitionResponse{}
	err = json.Unmarshal([]byte(entry.MetaData), &newFlowReq)
	if err != nil {
		DATASTACKAgent.Logger.Error(messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FLOWCREATIONERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		return
	}

	mirrorPath := ""
	if newFlowReq.Mirror {
		if rootFolderHashMap[entry.FlowID][inputFolder] == inputFolder {
			mirrorPath = filepath.Base(inputFolder)
		} else {
			rootPath := rootFolderHashMap[entry.FlowID][inputFolder]
			rootFolder := filepath.Base(rootPath)
			mirrorFolderArray := strings.Split(inputFolder, rootPath)
			mirrorPath = rootFolder + mirrorFolderArray[len(mirrorFolderArray)-1]

		}
	}

	//SetUpFlowProperties Object
	properties := models.FlowWatcherProperties{}
	properties.AppName = entry.AppName
	properties.AgentName = DATASTACKAgent.AgentName
	properties.FlowName = values["flow-name"]
	properties.FlowID = entry.FlowID
	properties.BlockName = newFlowReq.BlockName
	properties.StructureID = newFlowReq.StructureID
	properties.UniqueRemoteTxn = newFlowReq.UniqueRemoteTransaction
	properties.UniqueRemoteTxnOptions = newFlowReq.UniqueRemoteTransactionOptions
	properties.FileExtensions = newFlowReq.FileExtensions
	properties.FileNameRegexes = newFlowReq.FileNameRegexs
	properties.OutputDirectories = newFlowReq.OutputDirectory
	properties.TargetAgentID = newFlowReq.TargetAgentID
	properties.MirrorEnabled = newFlowReq.Mirror

	if uploadType == pollingUploads {
		properties.Listener = true
	}

	var inputFiles []os.FileInfo
	if uploadType == timeBoundUploads {
		file, err := os.Stat(inputFolder + string(os.PathSeparator) + newFlowReq.FileSuffix)
		if err != nil {
			DATASTACKAgent.Logger.Error("Error in init existing uploads - ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.PREPROCESSINGFILEERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
			return
		}
		inputFiles = append(inputFiles, file)
	} else {
		inputFiles, err = ioutil.ReadDir(inputFolder)
		if err != nil {
			DATASTACKAgent.Logger.Error("Error in init existing uploads - ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.PREPROCESSINGFILEERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
			return
		}
	}

	for _, inputFile := range inputFiles {
		if !inputFile.IsDir() && inputFile.Name() != "" {
			_, err := regexp.MatchString(values["file-suffix"], inputFile.Name())
			if err != nil {
				DATASTACKAgent.Logger.Error(fmt.Sprintf("%s", err))
			}
			matched := DATASTACKAgent.Utils.CheckFileRegexAndExtension(inputFile.Name(), newFlowReq.FileExtensions, newFlowReq.FileNameRegexs)
			result, ok := fileInUploadQueue.Load(inputFolder + string(os.PathSeparator) + inputFile.Name())
			if ok {
				ok = result.(bool)
			}
			if matched && !ok {
				fileInUploadQueue.Store(inputFolder+string(os.PathSeparator)+inputFile.Name(), true)
				_, originalFileName, originalAbsoluteFilePath, newFileName, newLocation, md5CheckSum, err := DATASTACKAgent.Utils.GetFileDetails(inputFolder+string(os.PathSeparator)+inputFile.Name(), DATASTACKAgent.AppFolderPath, entry.FlowName, newFlowReq.FileExtensions, newFlowReq.FileNameRegexs)
				DATASTACKAgent.Logger.Info("Existing File Name - %s, AbsoluteFilePath - %s", inputFolder+string(os.PathSeparator)+inputFile.Name(), originalAbsoluteFilePath)
				if err != nil && strings.Contains(fmt.Sprintf("%s", err), "The process cannot access the file because it is being used by another process") {
					fileInUploadQueue.Delete(inputFolder + string(os.PathSeparator) + inputFile.Name())
					continue
				} else if err != nil && (strings.Contains(messagegenerator.ExtractErrorMessageFromErrorObject(err), "The file name has more than 100 characters") || strings.Contains(messagegenerator.ExtractErrorMessageFromErrorObject(err), "The filesize is more than the permissible filesize")) {
					remoteTxnID := originalFileName
					u := uuid.NewV4()
					dataStackTxnID := u.String()
					flowFolder := DATASTACKAgent.AppFolderPath + string(os.PathSeparator) + strings.Replace(properties.FlowName, " ", "_", -1)
					newFileName = time.Now().Format("2006_01_02_15_04_05_000000") + "____" + originalFileName + ".error"
					newLocation = flowFolder + string(os.PathSeparator) + "error"
					DATASTACKAgent.addEntryToTransferLedger(properties.FlowName, properties.FlowID, ledgers.FILEUPLOADERRORDUETOLARGEFILENAMEORMAXFILESIZE, metadatagenerator.GenerateFileUploadErrorMetaDataForLargeFileNameOrMaxFileSize(originalFileName, originalAbsoluteFilePath, mirrorPath, properties.UniqueRemoteTxn, properties.UniqueRemoteTxnOptions.FileName, properties.UniqueRemoteTxnOptions.Checksum, properties.StructureID, properties.BlockName, properties.InputFolder, newFileName, newLocation, md5CheckSum, remoteTxnID, dataStackTxnID, messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
					os.Rename(inputFolder+string(os.PathSeparator)+inputFile.Name(), newLocation+string(os.PathSeparator)+newFileName)
					DATASTACKAgent.Utils.CreateFlowErrorFile(newLocation+string(os.PathSeparator)+dataStackTxnID+"_"+originalFileName+".Error.txt", messagegenerator.ExtractErrorMessageFromErrorObject(err))
					DATASTACKAgent.Logger.Error("Moving file to error folder - " + messagegenerator.ExtractErrorMessageFromErrorObject(err))
					continue
				} else if err != nil {
					remoteTxnID := originalFileName
					u := uuid.NewV4()
					dataStackTxnID := u.String()
					flowFolder := DATASTACKAgent.AppFolderPath + string(os.PathSeparator) + strings.Replace(properties.FlowName, " ", "_", -1)
					newFileName = time.Now().Format("2006_01_02_15_04_05_000000") + "____" + originalFileName + ".error"
					newLocation = flowFolder + string(os.PathSeparator) + "error"
					DATASTACKAgent.addEntryToTransferLedger(properties.FlowName, properties.FlowID, ledgers.WATCHERERROR, metadatagenerator.GenerateFileUploadErrorMetaDataForLargeFileNameOrMaxFileSize(originalFileName, originalAbsoluteFilePath, mirrorPath, properties.UniqueRemoteTxn, properties.UniqueRemoteTxnOptions.FileName, properties.UniqueRemoteTxnOptions.Checksum, properties.StructureID, properties.BlockName, properties.InputFolder, newFileName, newLocation, md5CheckSum, remoteTxnID, dataStackTxnID, messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
					os.Rename(inputFolder+string(os.PathSeparator)+inputFile.Name(), newLocation+string(os.PathSeparator)+newFileName)
					DATASTACKAgent.Utils.CreateFlowErrorFile(newLocation+string(os.PathSeparator)+dataStackTxnID+"_"+originalFileName+".Error.txt", messagegenerator.ExtractErrorMessageFromErrorObject(err))
					DATASTACKAgent.Logger.Error("Moving file to error folder - " + messagegenerator.ExtractErrorMessageFromErrorObject(err))
					fileInUploadQueue.Delete(inputFolder + string(os.PathSeparator) + inputFile.Name())
					continue
				}
				DATASTACKAgent.Logger.Info("Adding Upload Request In Queue for file %s", originalAbsoluteFilePath)
				u := uuid.NewV4()
				if err != nil {
					DATASTACKAgent.Logger.Error(fmt.Sprintf("%s", err))
				}
				remoteTxnID := originalFileName
				dataStackTxnID := u.String()
				DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.UPLOADREQUEST, metadatagenerator.GenerateFileUploadMetaData(originalFileName, originalAbsoluteFilePath, mirrorPath, uniqueRemoteTxn, uniqueRemoteTxnOptions.FileName, uniqueRemoteTxnOptions.Checksum, structureID, blockName, inputFolder, newFileName, newLocation, md5CheckSum, remoteTxnID, dataStackTxnID, properties), time.Now(), "IN", false)
			}
		} else if uploadType == pollingUploads {
			if weHaveToWatchSubDirectories[entry.FlowID][inputFolder] {
				value, ok := watchingDirectoryMap[entry.FlowID][inputFolder+string(os.PathSeparator)+inputFile.Name()]
				if ok {
					ok = value
				}

				if !ok && DATASTACKAgent.isInputDirectoryAvailable(inputFolder+string(os.PathSeparator)+inputFile.Name(), entry.FlowName) {
					weHaveToWatchSubDirectories[entry.FlowID][inputFolder+string(os.PathSeparator)+inputFile.Name()] = true
					watchingDirectoryMap[entry.FlowID][inputFolder+string(os.PathSeparator)+inputFile.Name()] = true
					DATASTACKAgent.handleGetMirrorFolderStructure(entry)
					if DATASTACKAgent.WatchersRunning.Value() < 1024 {
						DATASTACKAgent.WatchersRunning.Add(1)
						DATASTACKAgent.Logger.Info("Consumed %v/1024 watchers", DATASTACKAgent.WatchersRunning.Value())
						properties.InputFolder = inputFolder + string(os.PathSeparator) + inputFile.Name()
						go DATASTACKAgent.pollInputDirectoryEveryXSeconds(entry, properties)
					} else {
						DATASTACKAgent.Logger.Info("Cannot start watchers already 1024 watchers are running")
					}
				}
			}
		}
	}
}

func (DATASTACKAgent *AgentDetails) traverseThroughTheInputDirectoryStructure(rootPath string, flowID string) []string {
	var subFolderPaths []string
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			value, contains := watchingDirectoryMap[flowID][path]
			if !contains {
				subFolderPaths = append(subFolderPaths, path)
				watchingDirectoryMap[flowID][path] = true
			} else if !value {
				watchingDirectoryMap[flowID][path] = true
			}
			weHaveToWatchSubDirectories[flowID][path] = true
		}
		return nil
	})
	if err != nil {
		DATASTACKAgent.Logger.Error(err)
	}
	return subFolderPaths
}

//CompactDBHandler - reduce the size of transferledger.DB
func (DATASTACKAgent *AgentDetails) CompactDBHandler() {
	c := cron.New()
	c.AddFunc(DBSizeUpdateTimeRegEx, func() {
		DATASTACKAgent.Logger.Info("Agent is in maintainence state.")
		DATASTACKAgent.Paused = true
		for appName, flowChannels := range DATASTACKAgent.FlowsChannel {
			for flowID, channel := range flowChannels {
				count, ok := numberOfInputDirectoriesForFlow.Load(flowID)
				if ok {
					for i := 0; i < count.(int); i++ {
						channel <- "Closing listener"
					}
					DATASTACKAgent.WatchersRunning.Add(-1 * count.(int))
				}
				numberOfInputDirectoriesForFlow.Delete(flowID)
				delete(DATASTACKAgent.Flows[appName], flowID)
				delete(DATASTACKAgent.FlowsChannel[appName], flowID)
				delete(weHaveToWatchSubDirectories, flowID)
				delete(rootFolderHashMap, flowID)
				delete(watchingDirectoryMap, flowID)
			}
		}
		fileInUploadQueue = sync.Map{}
		DATASTACKAgent.TransferLedger.CompactDB()
		DATASTACKAgent.getRunningOrPendingFlowsFromB2BManager()
		DATASTACKAgent.Paused = false
		DATASTACKAgent.Logger.Info("Agent resumes, all operations started again.")
	})
	DATASTACKAgent.Logger.Info("Starting CompactDB Handler ...")
	c.Start()
}

func (DATASTACKAgent *AgentDetails) handleQueuedJobs(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			var err error
			transferLedgerEntries := []models.TransferLedgerEntry{}
			if !DATASTACKAgent.Paused {
				transferLedgerEntries, err = DATASTACKAgent.TransferLedger.GetQueuedOperations(10)
				if err != nil {
					DATASTACKAgent.addEntryToTransferLedger("", "", ledgers.QUEUEDJOBSERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
					continue
				}
			}
			if !DATASTACKAgent.Paused {
				for _, entry := range transferLedgerEntries {
					err = DATASTACKAgent.TransferLedger.UpdateSentOrReadFieldOfEntry(&entry, true)
					if err != nil {
						DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.QUEUEDJOBSERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
						continue
					}
					switch entry.Action {
					case ledgers.FLOWCREATEREQUEST:
						go DATASTACKAgent.handleFlowCreateStartOrUpdateRequest(entry)
						return
					case ledgers.FLOWSTARTREQUEST:
						go DATASTACKAgent.handleFlowCreateStartOrUpdateRequest(entry)
					case ledgers.FLOWUPDATEREQUEST:
						go DATASTACKAgent.handleFlowCreateStartOrUpdateRequest(entry)
						return
					case ledgers.FLOWSTOPREQUEST:
						go DATASTACKAgent.handleFlowStopRequest(entry)
						return
						// case ledgers.FILEPROCESSEDSUCCESS:
						// 	go DATASTACKAgent.handleFileProcessedSuccessRequest(entry)
						// 	return
						// case ledgers.FILEPROCESSEDERROR:
						// 	go DATASTACKAgent.handleFileProcessedFailureRequest(entry)
						// 	return
					}
				}
			}
			time.Sleep(time.Duration(2) * time.Second)
		}
	}()
}

//QueuedFileUploadWatcher - Uploading queued file
func (DATASTACKAgent *AgentDetails) QueuedFileUploadWatcher() {
	c := cron.New()
	c.AddFunc("@every 10m", func() {
		currentTime := time.Now()
		DATASTACKAgent.Logger.Debug("QueuedFileUploadWatcher time - %s", currentTime)
		for file, queueData := range QueuedFiles {
			DATASTACKAgent.Logger.Info("Queued File - %s %s", file, queueData)
			DATASTACKAgent.initExistingUploadsFromInputFolder(queueData.FlowProperties.InputFolder, queueData.Entry, queueData.FlowProperties.BlockName, queueData.FlowProperties.StructureID, queueData.FlowProperties.UniqueRemoteTxn, queueData.FlowProperties.UniqueRemoteTxnOptions, timeBoundUploads, queueData.FlowProperties.ErrorBlocks)
			delete(QueuedFiles, file)
		}
	})
	DATASTACKAgent.Logger.Info("Started Queued File Upload watcher RegEx %s", "@every 10m")
	c.Start()
}

func (DATASTACKAgent *AgentDetails) handleUploadFileRequest(entry models.TransferLedgerEntry, blockOpener chan bool) {
	var totalChunks string
	var totalNumberOfFileChunks int
	retryCount, err := strconv.Atoi(DATASTACKAgent.UploadRetryCounter)
	if err != nil {
		retryCount = 10
	}
	encryptedFileChecksum := ""
	decryptedFileChecksum := ""
	encryptionKey := DATASTACKAgent.EncryptionKey

	flowFolder := DATASTACKAgent.AppFolderPath + string(os.PathSeparator) + strings.Replace(entry.FlowName, " ", "_", -1)
	errorFolder := flowFolder + string(os.PathSeparator) + "error"

	fileUploadMetaData := models.FileUploadMetaData{}
	err = json.Unmarshal([]byte(entry.MetaData), &fileUploadMetaData)
	fileUploadMetaData.FlowID = entry.FlowID
	fileUploadMetaData.OriginalFilePath = returnAbsolutePath(fileUploadMetaData.OriginalFilePath)
	fileUploadMetaData.FlowID = entry.FlowID

	if err != nil {
		DATASTACKAgent.Logger.Error(fmt.Sprintf("%s", err))
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.UPLOADERROR, metadatagenerator.GenerateFileUploadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileUploadMetaData.RemoteTxnID, fileUploadMetaData.DataStackTxnID), time.Now(), "OUT", false)
		fileInUploadQueue.Delete(fileUploadMetaData.OriginalFilePath)
		blockOpener <- true
		return
	}

	headers := make(map[string]string)
	fileUploadMetaData.AgentID = DATASTACKAgent.AgentID
	headers["DATA-STACK-Txn-ID"] = fileUploadMetaData.DataStackTxnID
	headers["DATA-STACK-Remote-Txn-ID"] = fileUploadMetaData.RemoteTxnID
	fileUploadMetaData.Token = DATASTACKAgent.Token

	DATASTACKAgent.Logger.Info("Handling File Upload Request %s", entry.FlowName)
	DATASTACKAgent.Logger.Debug("entry details -: %v", entry)
	DATASTACKAgent.Logger.Debug("Flow Retry Count -: %v", retryCount)

	for i := 0; i < retryCount; i++ {
		fileInfo, err := os.Stat(fileUploadMetaData.OriginalFilePath)
		if err != nil {
			DATASTACKAgent.Logger.Error(err)
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.UPLOADERROR, metadatagenerator.GenerateFileUploadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileUploadMetaData.RemoteTxnID, fileUploadMetaData.DataStackTxnID), time.Now(), "OUT", false)
			continue
		}

		fileSize := fileInfo.Size()
		fileSizeString := strconv.FormatInt(fileSize, 10)
		checksum, err := DATASTACKAgent.Utils.CalculateMD5ChecksumForFile(fileUploadMetaData.OriginalFilePath)
		if err != nil {
			DATASTACKAgent.Logger.Error("Calculate checksum error %v", err)
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.UPLOADERROR, metadatagenerator.GenerateFileUploadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileUploadMetaData.RemoteTxnID, fileUploadMetaData.DataStackTxnID), time.Now(), "OUT", false)
			continue
		}

		encryptionDone := false
		fileUploadMetaData.Md5CheckSum = checksum
		decryptedFileChecksum = checksum

		interactionMetaDataString := metadatagenerator.GenerateFileUploadedInteractionMetaData(fileUploadMetaData.OriginalFilePath, fileUploadMetaData.BlockName, fileSizeString, fileUploadMetaData.OriginalFileName, "1", fileUploadMetaData.StructureID, fileUploadMetaData.RemoteTxnID, fileUploadMetaData.DataStackTxnID, fileUploadMetaData.Md5CheckSum, DATASTACKAgent.IPAddress, DATASTACKAgent.MACAddress, DATASTACKAgent.EncryptFile, "", 0, 0, fileUploadMetaData.MirrorPath, runtime.GOOS)

		//Calculating total number of chunks to be sent to bm
		if totalChunks == "" {
			totalNumberOfFileChunks = int(math.Ceil(float64(fileSize) / float64(MaxChunkSize)))
		} else {
			totalNumberOfFileChunks, _ = strconv.Atoi(totalChunks)
		}

		tempTime := time.Now()
		//Code for sending the file to bm
		err = DATASTACKAgent.SendFileInChunksToBM(entry, fileUploadMetaData, encryptionDone, fileSize, encryptedFileChecksum, decryptedFileChecksum, totalNumberOfFileChunks, retryCount, i, flowFolder, errorFolder, encryptionKey)
		if err != nil {
			DATASTACKAgent.Logger.Error(err)
			fileInUploadQueue.Delete(fileUploadMetaData.OriginalFilePath)
			blockOpener <- true
			return
		} else {
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.UPLOADED, interactionMetaDataString, tempTime, "OUT", false)
			DATASTACKAgent.Logger.Info("Successfully uploaded file " + fileUploadMetaData.OriginalFilePath + " for " + fileUploadMetaData.DataStackTxnID)
			fileInUploadQueue.Delete(fileUploadMetaData.OriginalFilePath)
			blockOpener <- true
			return
		}
	}
	os.Rename(fileUploadMetaData.OriginalFilePath, errorFolder+string(os.PathSeparator)+fileUploadMetaData.OriginalFileName)
	fileInUploadQueue.Delete(fileUploadMetaData.OriginalFilePath)
	blockOpener <- true
}

//SendFileInChunksToBM - sending file in chunks chunks
func (DATASTACKAgent *AgentDetails) SendFileInChunksToBM(entry models.TransferLedgerEntry, fileUploadMetaData models.FileUploadMetaData, encryptionDone bool, totalFileSize int64, encryptedChecksum string, decryptedChecksum string, totalChunks int, retryCounter int, totalRetriesDone int, flowFolder string, errorFolder string, password string) error {
	var buffer []byte
	var binSize string
	var uploadError error
	var decryptedBuffer []byte
	var chunkSize int64
	var resMessage string
	lengthReadBuffer := make([]byte, 64)

	uuid := uuid.NewV4()
	uniqueID := uuid.String()

	if totalChunks > 1 {
		DATASTACKAgent.Logger.Info("File size greater than 100MB will upload the file in chunks")
		DATASTACKAgent.Logger.Info("Total chunks ", totalChunks)
	}

	for totalRetriesDone < retryCounter {
		DATASTACKAgent.Logger.Debug("Upload try count ", totalRetriesDone+1)
		DATASTACKAgent.Logger.Info("Started file upload process %s for flow %s", fileUploadMetaData.OriginalFileName, entry.FlowName)
		inputFile, err := os.Open(fileUploadMetaData.OriginalFilePath)
		if err != nil {
			uploadError = err
			DATASTACKAgent.Logger.Error("Input file Upload error ", err)
			continue
		}
		processingFile, err := os.Create(fileUploadMetaData.NewLocation)
		if err != nil {
			uploadError = err
			DATASTACKAgent.Logger.Error("Error creating in processing file ", err)
			continue
		}

		for i := 1; i <= totalChunks; i++ {
			if encryptionDone {
				_, err = inputFile.Read(lengthReadBuffer)
				if err != nil {
					inputFile.Close()
					os.Rename(fileUploadMetaData.OriginalFilePath, errorFolder+string(os.PathSeparator)+fileUploadMetaData.OriginalFileName)
					os.Remove(fileUploadMetaData.NewLocation)
					return err
				}
				chunkSize, _ = strconv.ParseInt(string(lengthReadBuffer), 2, 10)
				buffer = make([]byte, chunkSize)
				_, err = inputFile.Read(buffer)
				if err != nil {
					inputFile.Close()
					os.Rename(fileUploadMetaData.OriginalFilePath, errorFolder+string(os.PathSeparator)+fileUploadMetaData.OriginalFileName)
					os.Remove(fileUploadMetaData.NewLocation)
					return err
				}
				processingFile.Write(lengthReadBuffer)
				processingFile.Write(buffer)
			} else {
				bufferSize := getBufferSize(totalFileSize, MaxChunkSize)
				if bufferSize == 0 {
					inputFile.Close()
					os.Rename(fileUploadMetaData.OriginalFilePath, errorFolder+string(os.PathSeparator)+fileUploadMetaData.OriginalFileName)
					os.Remove(fileUploadMetaData.NewLocation)
					bufferSizeError := "Invalid buffer size while reading the file - " + strconv.FormatInt(bufferSize, 10)
					return errors.New(bufferSizeError)
				}

				decryptedBuffer = make([]byte, bufferSize)
				_, err := inputFile.Read(decryptedBuffer)
				if err != nil && err != io.EOF {
					inputFile.Close()
					os.Rename(fileUploadMetaData.OriginalFilePath, errorFolder+string(os.PathSeparator)+fileUploadMetaData.OriginalFileName)
					os.Remove(fileUploadMetaData.NewLocation)
					return err
				}
				compressedData := DATASTACKAgent.Utils.Compress(decryptedBuffer)
				buffer, err = DATASTACKAgent.Utils.EncryptData(compressedData, password)
				if err != nil {
					DATASTACKAgent.Logger.Error("Encryption Error ", err)
					return err
				}

				binSize = DATASTACKAgent.Utils.Get64BitBinaryStringNumber(int64(len(buffer)))
				processingFile.Write([]byte(binSize))
				processingFile.Write(buffer)
				totalFileSize -= bufferSize
				decryptedBuffer = nil
			}

			DATASTACKAgent.Logger.Trace("buffer %v", buffer)
			DATASTACKAgent.Logger.Trace("buffer %v", string(buffer))
			bufferCheckSum := DATASTACKAgent.Utils.CalculateMD5ChecksumForByteSlice(buffer)
			DATASTACKAgent.Logger.Info("Chunk length %v", len(buffer))
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.UPLOADING, entry.MetaData, time.Now(), "OUT", false)

			r, w := io.Pipe()
			writer := multipart.NewWriter(w)
			go func() {
				defer w.Close()
				defer writer.Close()
				part, err := writer.CreateFormFile("file", fileUploadMetaData.OriginalFilePath)
				if err != nil {
					DATASTACKAgent.Logger.Error("Multipart File creation error - ", err)
					return
				}
				part.Write(buffer)
			}()

			for totalRetriesDone < retryCounter {
				var client *http.Client
				client = DATASTACKAgent.FetchHTTPClient()
				DATASTACKAgent.Logger.Info("Uploading chunk %v/%v  of  %s for flow  %s", i, totalChunks, fileUploadMetaData.OriginalFileName, entry.FlowName)
				URL := DATASTACKAgent.BaseURL + "/b2b/bm/{app}/agent/utils/{agentId}/upload"
				req, _ := http.NewRequest("POST", URL, r)
				req.Header.Set("DATA-STACK-Agent-Id", DATASTACKAgent.AgentID)
				req.Header.Set("DATA-STACK-Agent-Name", DATASTACKAgent.AgentName)
				req.Header.Set("DATA-STACK-App-Name", DATASTACKAgent.AppName)
				req.Header.Set("DATA-STACK-Flow-Name", entry.FlowName)
				req.Header.Set("DATA-STACK-Flow-Id", entry.FlowID)
				req.Header.Set("DATA-STACK-New-File-Location", fileUploadMetaData.NewLocation)
				req.Header.Set("DATA-STACK-Original-File-Name", fileUploadMetaData.OriginalFileName)
				req.Header.Set("DATA-STACK-New-File-Name", fileUploadMetaData.NewFileName)
				req.Header.Set("DATA-STACK-File-Checksum", fileUploadMetaData.Md5CheckSum)
				req.Header.Set("DATA-STACK-Remote-Txn-Id", fileUploadMetaData.RemoteTxnID)
				req.Header.Set("DATA-STACK-Txn-Id", fileUploadMetaData.DataStackTxnID)
				req.Header.Set("DATA-STACK-Deployment-Name", entry.DeploymentName)
				req.Header.Set("DATA-STACK-File-Size", strconv.Itoa(int(totalFileSize)))
				req.Header.Set("DATA-STACK-Symmetric-Key", password)
				req.Header.Set("DATA-STACK-Mirror-Directory", fileUploadMetaData.MirrorPath)
				req.Header.Set("DATA-STACK-Operating-System", runtime.GOOS)
				req.Header.Set("DATA-STACK-Total-Chunks", strconv.Itoa(totalChunks))
				req.Header.Set("DATA-STACK-Current-Chunk", strconv.Itoa(i))
				req.Header.Set("DATA-STACK-Unique-ID", uniqueID)
				req.Header.Set("DATA-STACK-BufferEncryption", "true")
				req.Header.Set("DATA-STACK-Agent-Release", DATASTACKAgent.Release)
				req.Header.Set("DATA-STACK-Chunk-Checksum", bufferCheckSum)
				req.Header.Set("DATA-STACK-Compression", "true")
				req.Header.Set("DATA-STACK-File-Token", fileUploadMetaData.Token)
				req.Header.Set("Authorization", "JWT "+DATASTACKAgent.Token)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				DATASTACKAgent.Logger.Info("Making Request at -: %s", URL)
				DATASTACKAgent.Logger.Debug("Request Headers -: %s", req.Header)
				req.Close = true
				res, err := client.Do(req)
				if err != nil {
					uploadError = err
					DATASTACKAgent.Logger.Error("File Upload error ", err)
					totalRetriesDone++
				} else {
					var message []byte
					if res.Body != nil {
						message, _ = ioutil.ReadAll(res.Body)
					}
					if strings.Contains(string(message), messagegenerator.InvalidIP) {
						inputFile.Close()
						processingFile.Close()
						os.Rename(fileUploadMetaData.OriginalFilePath, errorFolder+string(os.PathSeparator)+fileUploadMetaData.OriginalFileName)
						os.Remove(fileUploadMetaData.NewLocation)
						DATASTACKAgent.Logger.Error("IP is not trusted, stopping the agent")
						os.Exit(0)
					}

					if string(message) != "" && (string(message) != "File Successfully Uploaded") || (string(message) != "Chunk Successfully Uploaded") {
						DATASTACKAgent.Logger.Error(message)
						resMessage = string(message)
					}

					if res.StatusCode != 200 {
						DATASTACKAgent.Logger.Error("File Upload error with status code - ", res.StatusCode)
						DATASTACKAgent.Logger.Error("Chunk Upload fails trying again")
						totalRetriesDone++
						client = nil
						req = nil

						if res.Body != nil {
							res.Body.Close()
						}
						res = nil
					} else {
						DATASTACKAgent.Logger.Info("Chunk %v/%v of %s for flow %s uploaded successfully", i, totalChunks, fileUploadMetaData.OriginalFileName, entry.FlowName)
						buffer = nil
						client = nil
						req = nil
						if res.Body != nil {
							res.Body.Close()
						}
						res = nil
						break
					}
				}

			}

			if totalRetriesDone >= retryCounter {
				break
			}

			if i == totalChunks {
				inputFile.Close()
				processingFile.Close()
				os.Remove(fileUploadMetaData.OriginalFilePath)
				return nil
			}

		}
		if totalRetriesDone >= retryCounter {
			inputFile.Close()
			processingFile.Close()
			os.Rename(fileUploadMetaData.OriginalFilePath, errorFolder+string(os.PathSeparator)+fileUploadMetaData.OriginalFileName)
			os.Remove(fileUploadMetaData.NewLocation)
			fileUploadMetaData.AgentID = DATASTACKAgent.AgentID
			byteData, _ := json.Marshal(fileUploadMetaData)
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.FILEUPLOADTOBMERROR, string(byteData), time.Now(), "IN", false)
			uploadError = errors.New("File Upload to BM failed for file - " + fileUploadMetaData.RemoteTxnID)
			if resMessage == "" {
				resMessage = "File Upload to BM failed for file - " + fileUploadMetaData.RemoteTxnID
			}
			DATASTACKAgent.Utils.CreateFlowErrorFile(errorFolder+string(os.PathSeparator)+fileUploadMetaData.DataStackTxnID+"_"+fileUploadMetaData.OriginalFileName+".Error.txt", resMessage)
			break
		}
		totalRetriesDone++
	}
	return uploadError
}

func getBufferSize(totalSizeLeft int64, MaxChunkSize int) int64 {
	if totalSizeLeft >= int64(MaxChunkSize) {
		return int64(MaxChunkSize)
	} else {
		return totalSizeLeft
	}
}

func (DATASTACKAgent *AgentDetails) processQueuedUploads() {
	for {
		var err error
		transferLedgerEntries := []models.TransferLedgerEntry{}
		if !DATASTACKAgent.Paused {
			transferLedgerEntries, err = DATASTACKAgent.TransferLedger.GetFileUploadRequests(DATASTACKAgent.MaxConcurrentUploads)
			if err != nil {
				DATASTACKAgent.addEntryToTransferLedger("", "", ledgers.QUEUEDJOBSERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
				continue
			}
		}
		queueFilesCount := len(transferLedgerEntries)
		DATASTACKAgent.Logger.Info("Queuefilescount - ", queueFilesCount)
		if queueFilesCount == 0 {
			time.Sleep(time.Duration(1) * time.Second)
		} else if !DATASTACKAgent.Paused {
			DATASTACKAgent.FilesUploading += int64(queueFilesCount)
			blockChannel := make(chan bool, queueFilesCount)
			for _, entry := range transferLedgerEntries {
				err = DATASTACKAgent.TransferLedger.UpdateSentOrReadFieldOfEntry(&entry, true)
				if err != nil {
					DATASTACKAgent.addEntryToTransferLedger("", "", ledgers.QUEUEDJOBSERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
					continue
				} else {
					go DATASTACKAgent.handleUploadFileRequest(entry, blockChannel)
				}
			}
			for i := 0; i < queueFilesCount; i++ {
				<-blockChannel
				DATASTACKAgent.FilesUploading--
			}
			close(blockChannel)
			DATASTACKAgent.Logger.Debug("Upload Block Opened for Count -: ", queueFilesCount)
		}
	}
}

func (DATASTACKAgent *AgentDetails) processQueuedDownloads() {
	DATASTACKAgent.Logger.Info("Started Queued Downloads")
	for {
		var err error
		transferLedgerEntries := []models.TransferLedgerEntry{}

		if !DATASTACKAgent.Paused {
			transferLedgerEntries, err = DATASTACKAgent.TransferLedger.GetFileDownloadRequests(DATASTACKAgent.MaxConcurrentDownloads)
			if err != nil {
				DATASTACKAgent.addEntryToTransferLedger("", "", ledgers.QUEUEDJOBSERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
				continue
			}
		}

		queueFilesCount := len(transferLedgerEntries)
		if queueFilesCount == 0 {
			time.Sleep(time.Duration(1) * time.Second)
		} else if !DATASTACKAgent.Paused {
			DATASTACKAgent.FilesDownloading += int64(queueFilesCount)
			DATASTACKAgent.Logger.Info("Fetched Queued Downloads -: %v", queueFilesCount)
			blockDownloadChannel := make(chan bool, queueFilesCount)
			for _, entry := range transferLedgerEntries {
				err = DATASTACKAgent.TransferLedger.UpdateSentOrReadFieldOfEntry(&entry, true)
				if err != nil {
					DATASTACKAgent.addEntryToTransferLedger("", "", ledgers.QUEUEDJOBSERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
					continue
				} else {
					DATASTACKAgent.handleDownloadFileRequest(entry, blockDownloadChannel)
				}
			}
			for i := 0; i < queueFilesCount; i++ {
				<-blockDownloadChannel
				DATASTACKAgent.FilesDownloading--
			}
			close(blockDownloadChannel)
			DATASTACKAgent.Logger.Info("Download block open for count -: %v", queueFilesCount)
		}
	}
}

func (DATASTACKAgent *AgentDetails) handleDownloadFileRequest(entry models.TransferLedgerEntry, blockOpener chan bool) {

	fileDownloadMetaData := models.DownloadFileRequestMetaData{}
	err := json.Unmarshal([]byte(entry.MetaData), &fileDownloadMetaData)
	if err != nil {
		DATASTACKAgent.Logger.Error("file download error-1 json unmarshalling error %s ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
		blockOpener <- true
		return
	}

	if DATASTACKAgent.TransferLedger.IsFileAlreadyDownloaded(fileDownloadMetaData.FileID) {
		blockOpener <- true
		return
	}
	DATASTACKAgent.Logger.Info("Started file download for Flow-: %v, FlowId-: %v", entry.FlowName, entry.FlowID)
	DATASTACKAgent.Logger.Debug("entry details %s", entry.MetaData)

	var files []*os.File
	mirrorOutputPath := fileDownloadMetaData.MirrorDirectory
	if fileDownloadMetaData.OperatingSystem == "windows" && runtime.GOOS != "windows" {
		mirrorOutputPath = strings.Replace(mirrorOutputPath, "\\", "/", -1)

	} else if fileDownloadMetaData.OperatingSystem != "windows" && runtime.GOOS == "windows" {
		mirrorOutputPath = strings.Replace(mirrorOutputPath, "/", "\\", -1)

	}

	if fileDownloadMetaData.HeaderOutputDirectory != "" {
		path, _ := filepath.Abs(fileDownloadMetaData.HeaderOutputDirectory)
		if path[0] == '\\' && path[1] != '\\' {
			path = string(os.PathSeparator) + path
		}
		DATASTACKAgent.Logger.Info("Starting Download file process for file " + fileDownloadMetaData.FileName + " to " + path)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			DATASTACKAgent.Logger.Error("Header Output Directory "+path+" Doesn't Exist , Create Output Directory And Restart the Agent error message %s", err)
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.OUTPUTDIRECTORYDOESNOTEXISTERROR, metadatagenerator.GenerateFileDownloadErrorMetaData("Output Directory Doesn't Exist , Create Output Directory And Restart the Agent", fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
			blockOpener <- true
			return

		} else {
			file, err := os.Create(path + string(os.PathSeparator) + fileDownloadMetaData.FileName)
			if err != nil {
				DATASTACKAgent.Logger.Error("file download error - %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
				DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
				blockOpener <- true
				return
			}
			files = append(files, file)
		}
	} else if len(fileDownloadMetaData.FileLocation) != 0 {
		for _, dir := range fileDownloadMetaData.FileLocation {
			path, _ := filepath.Abs(dir)
			if path[0] == '\\' && path[1] != '\\' {
				path = string(os.PathSeparator) + path
			}

			if mirrorOutputPath != "" {
				path = path + string(os.PathSeparator) + mirrorOutputPath
			}
			DATASTACKAgent.Logger.Info("Started Downloading file process for file  " + fileDownloadMetaData.FileName + " to " + path)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				DATASTACKAgent.Logger.Error("Output Directory "+path+" Doesn't Exist , Create Output Directory And Restart the Agent, error message is ", err)
				DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.OUTPUTDIRECTORYDOESNOTEXISTERROR, metadatagenerator.GenerateFileDownloadErrorMetaData("Output Directory Doesn't Exist , Create Output Directory And Restart the Agent", fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
				continue
			} else {
				file, err := os.Create(path + string(os.PathSeparator) + fileDownloadMetaData.FileName)
				if err != nil {
					DATASTACKAgent.Logger.Error("file download error %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
					DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
					continue
				}

				if err != nil {
					DATASTACKAgent.Logger.Error("file download error %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
					DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
					continue
				}
				files = append(files, file)
			}

		}

	} else {
		flowFolder := DATASTACKAgent.AppFolderPath + string(os.PathSeparator) + strings.Replace(entry.FlowName, " ", "_", -1)
		dir := flowFolder + string(os.PathSeparator) + "output"
		path, _ := filepath.Abs(dir)
		if mirrorOutputPath != "" {
			path = path + string(os.PathSeparator) + mirrorOutputPath
		}
		path = path + string(os.PathSeparator) + fileDownloadMetaData.FileName

		fileDownloadMetaData.FileLocation = append(fileDownloadMetaData.FileLocation, path)
		file, err := os.Create(path)
		DATASTACKAgent.Logger.Info("Started Downloading file process for file " + fileDownloadMetaData.FileName + " to " + path)
		if err != nil {
			DATASTACKAgent.Logger.Error("file download error %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
			blockOpener <- true
			return
		}
		files = append(files, file)
	}

	if len(files) == 0 {
		blockOpener <- true
		DATASTACKAgent.Logger.Error("No files/directories found to download the file")
		return
	}

	//DownloadProcess via http call to b2bgw or ieg
	retryCount, err := strconv.Atoi(DATASTACKAgent.DownloadRetryCounter)
	if err != nil {
		retryCount = 10
	}

	var client *http.Client
	data := models.DownloadFileRequest{
		AgentName:    DATASTACKAgent.AgentName,
		AgentID:      DATASTACKAgent.AgentID,
		AgentVersion: strconv.FormatInt(DATASTACKAgent.AgentVersion, 10),
		FileName:     fileDownloadMetaData.FileName,
		FlowName:     entry.FlowName,
		FlowID:       entry.FlowID,
		AppName:      entry.AppName,
		FileID:       fileDownloadMetaData.FileID,
	}

	payload, err := json.Marshal(data)
	if err != nil {
		DATASTACKAgent.Logger.Error("file download error payload marshalling - %s ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
		RemoveFiles(files)
		blockOpener <- true
		return
	}

	var downloadFileError error
	var dataToWriteInFile []byte
	DATASTACKAgent.Logger.Trace("fileDownloadMetaData.ChunkChecksumList - ", fileDownloadMetaData.ChunkChecksumList)
	checksumsToVerify := strings.Split(fileDownloadMetaData.ChunkChecksumList, ",")
	DATASTACKAgent.Logger.Trace("checksumsToVerify - ", checksumsToVerify)
	currentChunk := 1
	TotalChunks, _ := strconv.Atoi(fileDownloadMetaData.TotalChunks)

	if TotalChunks > 1 {
		DATASTACKAgent.Logger.Info("File size is greater than 100MB, agent will download the file in chunks.")
	}

	for i := 0; i < retryCount; i++ {
		for currentChunk <= TotalChunks {
			DATASTACKAgent.Logger.Info("Downloading chunk %v/%v of %s for flow %s", currentChunk, TotalChunks, fileDownloadMetaData.FileName, entry.FlowName)
			client = DATASTACKAgent.FetchHTTPClient()
			URL := DATASTACKAgent.BaseURL + "/b2b/bm/{app}/agent/utils/{agentId}/download"
			req, err := http.NewRequest("POST", URL, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("DATA-STACK-Agent-File-Id", data.FileID)
			req.Header.Set("DATA-STACK-Agent-Id", DATASTACKAgent.AgentID)
			req.Header.Set("DATA-STACK-Agent-Name", DATASTACKAgent.AgentName)
			req.Header.Set("DATA-STACK-App-Name", DATASTACKAgent.AppName)
			req.Header.Set("DATA-STACK-Chunk-Number", strconv.Itoa(currentChunk))
			req.Header.Set("DATA-STACK-BufferEncryption", "true")
			req.Header.Set("Authorization", "JWT "+DATASTACKAgent.Token)
			if err != nil {
				downloadFileError = err
				DATASTACKAgent.Logger.Error("file download error-3 %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
				DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
				break
			}
			req.Close = true
			response, err := client.Do(req)
			if err != nil {
				downloadFileError = err
				DATASTACKAgent.Logger.Error("file download error while making request %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
				DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
				break
			} else if response.StatusCode == 200 {
				encryptedChunk, err := ioutil.ReadAll(response.Body)
				if response.Body != nil {
					response.Body.Close()
				}
				response = nil
				if err != nil {
					downloadFileError = err
					DATASTACKAgent.Logger.Error("file download error while reading response body %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
					break
				}
				DATASTACKAgent.Logger.Trace("encryptedChunk - ", encryptedChunk)
				DATASTACKAgent.Logger.Trace("encryptedChunk string - ", string(encryptedChunk))
				decodedBuffer, err := base64.StdEncoding.DecodeString(string(encryptedChunk))
				if err != nil {
					DATASTACKAgent.Logger.Error("decoding error = ", err)
					break
				}
				DATASTACKAgent.Logger.Trace("decoded buffer - ", decodedBuffer)
				DATASTACKAgent.Logger.Trace("decoded buffer string - ", string(decodedBuffer))

				chunkChecksum := DATASTACKAgent.Utils.CalculateMD5ChecksumForByteSlice(encryptedChunk)
				DATASTACKAgent.Logger.Trace("chunkChecksum - ", chunkChecksum)
				if checksumsToVerify[currentChunk-1] != chunkChecksum {
					downloadFileError = errors.New("Chunk Checksum match failed for download file - " + fileDownloadMetaData.FileName)
					DATASTACKAgent.Logger.Error("Checksum match failed for download file - " + fileDownloadMetaData.FileName)
					encryptedChunk = nil
					DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateErrorMessageMetaData("Checksum match failed for downloaded file - "+fileDownloadMetaData.FileName), time.Now(), "OUT", false)
					break
				} else if DATASTACKAgent.EncryptFile {
					dataToWriteInFile = encryptedChunk
					encryptedChunk = nil
				} else {
					decryptedData, err := DATASTACKAgent.Utils.DecryptData(decodedBuffer, fileDownloadMetaData.Password)
					DATASTACKAgent.Logger.Trace("compressedChunk - ", decryptedData)
					dataToWriteInFile = DATASTACKAgent.Utils.Decompress(decryptedData)
					DATASTACKAgent.Logger.Trace("dataToWriteInFile - ", dataToWriteInFile)
					if err != nil {
						downloadFileError = err
						DATASTACKAgent.Logger.Error("Decrypting chunk error -: ", err)
						encryptedChunk = nil
						decryptedData = nil
						break
					}
					decryptedData = nil
				}

				for i := 0; i < len(files); i++ {
					_, err = files[i].Write(dataToWriteInFile)
					if err != nil {
						DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
						DATASTACKAgent.Logger.Error("Writing download file error ", err)
						RemoveFiles(files)
						files = nil
						blockOpener <- true
						return
					}
				}

				dataToWriteInFile = nil

			} else {
				resp, err := ioutil.ReadAll(response.Body)
				if response.Body != nil {
					response.Body.Close()
				}
				if string(resp) == "file is already downloading" {
					DATASTACKAgent.Logger.Info("File is already downloading/downloaded")
					blockOpener <- true
					return
				}

				if strings.Contains(string(resp), messagegenerator.InvalidIP) {
					DATASTACKAgent.Logger.Error("IP is not trusted, stopping the agent")
					entry.SentOrRead = false
					DATASTACKAgent.TransferLedger.AddEntry(&entry)
					RemoveFiles(files)
					files = nil
					os.Exit(0)
				}

				if err != nil {
					DATASTACKAgent.Logger.Error(messagegenerator.ExtractErrorMessageFromErrorObject(err))
					DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
				} else {
					DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, string(resp), time.Now(), "OUT", false)
				}
				break
			}
			DATASTACKAgent.Logger.Info("File Chunk %v/%v of %s for flow %s downloaded successfully", currentChunk, TotalChunks, fileDownloadMetaData.FileName, entry.FlowName)
			currentChunk++
		}

		if currentChunk > TotalChunks {
			//File download complete sending downloaded interaction
			interactionChecksum, _ := DATASTACKAgent.Utils.CalculateMD5ChecksumForFile(files[0].Name())
			fileStat, _ := os.Stat(files[0].Name())
			fileSize := fileStat.Size()
			size := strconv.FormatInt(fileSize, 10)
			interactionMetadataString := metadatagenerator.GenerateFileDownloadedInteractionMetaData(fileDownloadMetaData.FileLocation, fileDownloadMetaData.BlockName, size, fileDownloadMetaData.FileName, fileDownloadMetaData.SequenceNo, fileDownloadMetaData.StructureID, fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID, interactionChecksum, DATASTACKAgent.IPAddress, DATASTACKAgent.MACAddress, DATASTACKAgent.EncryptFile, fileDownloadMetaData.SuccessBlock)
			if entry.Action == ledgers.REDOWNLOADFILEREQUEST {
				DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.REDOWNLOADED, string(interactionMetadataString), time.Now(), "OUT", false)
			} else {
				DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADED, string(interactionMetadataString), time.Now(), "OUT", false)
				if fileDownloadMetaData.SuccessBlock == "true" {
					// go DATASTACKAgent.handleSuccessFlowFileUploadRequest(entry)
				}
			}
			DATASTACKAgent.Logger.Info("%s file downloaded successfully", fileDownloadMetaData.FileName)
			DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADED+fileDownloadMetaData.FileID, "DOWNLOADED"+fileDownloadMetaData.FileID, time.Now(), "OUT", true)
			CloseFiles(files)
			files = nil
			blockOpener <- true
			return
		}
	}
	CloseFiles(files)
	DATASTACKAgent.addEntryToTransferLedger(entry.FlowName, entry.FlowID, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(downloadFileError), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
	blockOpener <- true
}

//RemoveFiles -: deleting list of files
func RemoveFiles(files []*os.File) {
	for i := 0; i < len(files); i++ {
		if files[i] != nil {
			os.Remove(files[i].Name())
		}
	}
}

//CloseFiles - closing files
func CloseFiles(files []*os.File) {
	for i := 0; i < len(files); i++ {
		if files[i] != nil {
			files[i].Close()
		}
	}
}
