package agent

import (
	"bytes"
	"ds-agent/ledgers"
	"ds-agent/log"
	"ds-agent/messagegenerator"
	"ds-agent/metadatagenerator"
	"ds-agent/models"
	"ds-agent/utils"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
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
	vault "github.com/appveen/govault"
	"github.com/gorilla/mux"
	"github.com/robfig/cron"
	uuid "github.com/satori/go.uuid"
)

const (
	//AlreadyRunningAgent - agent is already running or not
	AlreadyRunningAgent = "ALREADY_RUNNING"

	//DisabledAgent - agent is disabled
	DisabledAgent = "DISABLED"

	//VaultVersionChanged - vault version changed
	VaultVersionChanged = "VAULT_VERSION_CHANGED"

	//ReleaseChanged - release changed
	ReleaseChanged = "RELEASE_CHANGED"

	//vaultValues
	maxconcurrentuploads = "max-concurrent-uploads"

	//MaxChunkSize - for encryption and decryption, and it will be same forever we cannot change it otherwise there will be consequences.
	MaxChunkSize = 1024 * 1024 * 100

	//BytesToSkipWhileDecrypting - redundant bytes not actual file data
	BytesToSkipWhileDecrypting = 64

	//DBSizeUpdateTimeRegEx - time when agen will be in pause state
	DBSizeUpdateTimeRegEx = "@midnight"

	pollingUploads = "POLLING_UPLOADS"

	timeBoundUploads = "TIME_BOUND_UPLOADS"
)

//AgentService - task of agent
type AgentService interface {
	StartAgent() error
	getRunningOrPendingFlowFromIntegrationManagerAfterRestart()
	initCentralHeartBeat(wg *sync.WaitGroup)
	fetchAgentDetailsFromVault() error
	initAgentServer() error
	fetchHTTPClient() *http.Client
	addEntryToTransferLedger(namespace string, partnerName string, partnerID string, flowName string, flowID string, deploymentName string, action string, metaData string, time time.Time, entryType string, sentOrRead bool) error
	updateAgentConfFile() error
	updateAgentModeAndPortNumber(mode string) error
	updateHeaders(headers *http.Header)
	processQueuedDownloads()
	handleDownloadFileRequest(entry models.TransferLedgerEntry, blockOpener chan bool)
	encryptFileHandler(w http.ResponseWriter, r *http.Request)
	decryptFileHandler(w http.ResponseWriter, r *http.Request)
	handleSuccessFlowFileUploadRequest(entry models.TransferLedgerEntry)
	handleFlowCreateStartOrUpdateRequest(entry models.TransferLedgerEntry)
	handleFlowStopRequest(entry models.TransferLedgerEntry)
	createFlowFolderStructure(flowFolder string)
	handleGetMirrorFolderStructure(entry models.TransferLedgerEntry)
	generateMirrorDirectoryStructure(inputDirectory string, flowID string) []string
	createMirrorFolderStructureInDoneAndErrorFolder(flowName string, mirrorPaths []string)
	createMirrorOutputDirectoryStructure(mirrorOutputFolderPaths []string, outputDirectory string)
	determineCountOfInputDirectories(foldersToBeWatched []string, flowName string) int
	pollInputDirectoryEveryXSeconds(entry models.TransferLedgerEntry, partnerName string, flowName string, inputFolder string, flowID string, partnerID string, deploymentName string, namespace string, blockName string, structureID string, uniqueRemoteTxn bool, uniqueRemoteTxnOptions models.UniqueRemoteTransactionOptions, fileExtensions []models.FileExtensionStruct, fileNameRegexes []string)
	initExistingUploadsFromInputFolder(inputFolder string, entry models.TransferLedgerEntry, blockName string, structureID string, uniqueRemoteTxn bool, uniqueRemoteTxnOptions models.UniqueRemoteTransactionOptions, uploadType string, errorBlocks bool)
	handleErrorFlowRequest(entry models.TransferLedgerEntry, properties models.FlowWatcherProperties, remoteTxnID string, dataStackTxnID string, errMsg string)
}

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

//AgentDetails - details of agent
type AgentDetails struct {
	AgentID                string
	Mode                   string
	AgentName              string
	Password               string
	AgentVersion           string
	AppName                string
	UploadRetryCounter     string
	DownloadRetryCounter   string
	BaseURL                string
	HeartBeatFrequency     string
	AgentType              string
	LogLevel               string
	SentinelPortNumber     string
	SentinelMaxMissesCount string
	EncryptFile            string
	RetainFileOnSuccess    string
	RetainFileOnError      string
	AgentPortNumber        string
	VaultVersion           int
	Release                string
	MaxLogBackUpIndex      string
	LogMaxFileSize         string
	LogRotationType        string
	ExecutableDirectory    string
	CentralFolder          string
	VaultFilePath          string
	ConfFilePath           string
	AbsolutePath           string
	AppFolderPath          string
	TransferLedgerPath     string
	MonitoringLedgerPath   string
	DATASTACKCertFile      []byte
	DATASTACKKeyFile       []byte
	TrustCerts             []string
	Vault                  vault.VaultService
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
	Paused                 bool
	FilesUploading         int64
	FilesDownloading       int64
	WatchersRunning        watcherCounter
	Flows                  map[string]map[string]map[string]string
	FlowsChannel           map[string]map[string]chan string
}

//SetUpAgent -  setting up the datastack-agent
func SetUpAgent(centralFolder string, DATASTACKAgent *AgentDetails, pass string, interactive bool) error {
	var err error
	DATASTACKAgent.FilesUploading = 0
	DATASTACKAgent.FilesDownloading = 0
	DATASTACKAgent.Password = pass
	DATASTACKAgent.Utils = &utils.UtilsService{}
	DATASTACKAgent.InteractiveMode = interactive
	DATASTACKAgent.ExecutableDirectory, _, _ = utils.GetExecutablePathAndName()
	DATASTACKAgent.VaultFilePath = filepath.Join(DATASTACKAgent.ExecutableDirectory, "..", "conf", "db.vault")
	DATASTACKAgent.ConfFilePath = filepath.Join(DATASTACKAgent.ExecutableDirectory, "..", "conf", "agent.conf")
	DATASTACKAgent.CentralFolder = centralFolder
	DATASTACKAgent.AbsolutePath, _ = os.Getwd()
	DATASTACKAgent.Vault, err = vault.InitVault(DATASTACKAgent.VaultFilePath, pass)
	if err != nil {
		DATASTACKAgent.Logger.Error(fmt.Sprintf("Vault error -: %s", err))
		os.Exit(0)
	}
	DATASTACKAgent.fetchAgentDetailsFromVault()
	DATASTACKAgent.WatchersRunning = watcherCounter{}

	if DATASTACKAgent.BaseURL != "" && !strings.Contains(DATASTACKAgent.BaseURL, "https") {
		DATASTACKAgent.BaseURL = "https://" + DATASTACKAgent.BaseURL
	}

	logClient := DATASTACKAgent.fetchHTTPClient()
	logsHookURL := DATASTACKAgent.BaseURL + "/logs"
	headers := map[string]string{}
	headers["DATA-STACK-App-Name"] = DATASTACKAgent.AppName
	headers["DATA-STACK-Agent-Id"] = DATASTACKAgent.AgentID
	headers["DATA-STACK-Agent-Name"] = DATASTACKAgent.AgentName
	headers["DATA-STACK-Mac-Address"] = DATASTACKAgent.MACAddress
	headers["DATA-STACK-Ip-Address"] = DATASTACKAgent.IPAddress
	headers["AgentType"] = DATASTACKAgent.AgentType
	headers["DATA-STACK-Agent-Type"] = DATASTACKAgent.AgentType

	LoggerService := log.Logger{}
	DATASTACKAgent.Logger = LoggerService.GetLogger(DATASTACKAgent.AgentType, DATASTACKAgent.LogLevel, DATASTACKAgent.AgentName, DATASTACKAgent.AgentID, DATASTACKAgent.MaxLogBackUpIndex, DATASTACKAgent.LogMaxFileSize, DATASTACKAgent.LogRotationType, logsHookURL, logClient, headers)
	DATASTACKAgent.Logger.Info("Successfuly Logged in to vault")

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
	DATASTACKAgent.Paused = false
	DATASTACKAgent.Flows = make(map[string]map[string]map[string]string)
	DATASTACKAgent.FlowsChannel = make(map[string]map[string]chan string)
	DATASTACKAgent.Paused = false
	return nil
}

//StartAgent - Entry point function to invoke remote machine agent for file upload/download
func (DATASTACKAgent *AgentDetails) StartAgent() error {
	var wg sync.WaitGroup
	DATASTACKAgent.Logger.Info("Agent Starting ...")
	DATASTACKAgent.Logger.Debug("AgentID- %s", DATASTACKAgent.AgentID)
	DATASTACKAgent.Logger.Info("AgentName- %s", DATASTACKAgent.AgentName)
	DATASTACKAgent.Logger.Debug("Mode- %s", DATASTACKAgent.Mode)
	DATASTACKAgent.Logger.Debug("Agent Version- %s", DATASTACKAgent.AgentVersion)
	DATASTACKAgent.Logger.Info("Release version- %s", DATASTACKAgent.Release)
	DATASTACKAgent.Logger.Info("App Name- %s", DATASTACKAgent.AppName)
	DATASTACKAgent.Logger.Debug("Upload Retry Counter- %s", DATASTACKAgent.UploadRetryCounter)
	DATASTACKAgent.Logger.Debug("Download Retry Counter- %s", DATASTACKAgent.DownloadRetryCounter)
	DATASTACKAgent.Logger.Debug("Log Level- %s", DATASTACKAgent.LogLevel)
	DATASTACKAgent.Logger.Debug("Encrypt File- %s", DATASTACKAgent.EncryptFile)
	DATASTACKAgent.Logger.Debug("Retain File On Success- %s", DATASTACKAgent.RetainFileOnSuccess)
	DATASTACKAgent.Logger.Debug("Retain File On Error- %s", DATASTACKAgent.RetainFileOnError)
	DATASTACKAgent.Logger.Debug("Max Concurrent Uploads- %v", DATASTACKAgent.MaxConcurrentUploads)
	DATASTACKAgent.Logger.Debug("Max Concurrent Downloads- %v", DATASTACKAgent.MaxConcurrentDownloads)
	DATASTACKAgent.Logger.Debug("Vault Version- %v", DATASTACKAgent.VaultVersion)
	DATASTACKAgent.Logger.Debug("Absolute File Path- %s", DATASTACKAgent.AbsolutePath)
	DATASTACKAgent.Logger.Debug("Conf File Path- %s", DATASTACKAgent.ConfFilePath)
	DATASTACKAgent.Logger.Info("Encryption Type -: Buffered Encryption")
	DATASTACKAgent.Logger.Info("Interactive mode  %v", DATASTACKAgent.InteractiveMode)

	err := DATASTACKAgent.MonitoringLedger.AddOrUpdateEntry(&models.MonitoringLedgerEntry{
		AgentID:            DATASTACKAgent.AgentID,
		AgentName:          DATASTACKAgent.AgentName,
		AppName:            DATASTACKAgent.AppName,
		ResponseAgentID:    "",
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
	DATASTACKAgent.Logger.Info("Connecting to gateway - " + DATASTACKAgent.BaseURL)
	DATASTACKAgent.getRunningOrPendingFlowFromIntegrationManagerAfterRestart()
	DATASTACKAgent.Logger.Info("Maximum concurrent watcher limited to 1024.")
	DATASTACKAgent.Logger.Info("Starting central Heartbeat")
	go DATASTACKAgent.initCentralHeartBeat(&wg)
	DATASTACKAgent.Logger.Info("Starting Queue Downloads")
	go DATASTACKAgent.processQueuedDownloads()
	DATASTACKAgent.Logger.Debug("Starting Agent Server ...")
	go DATASTACKAgent.initAgentServer()
	DATASTACKAgent.CompactDBHandler()
	wg.Wait()
	return nil
}

//GetRunningORPendingFlowFromPartnerManagerAfterRestart - get details of running or pending flows associated with this agent
func (DATASTACKAgent *AgentDetails) getRunningOrPendingFlowFromIntegrationManagerAfterRestart() {
	DATASTACKAgent.Logger.Info("Fetching the flow(s) information")
	var client = DATASTACKAgent.fetchHTTPClient()

	data := models.CentralHeartBeatResponse{}
	headers := make(map[string]string)
	headers["AgentID"] = DATASTACKAgent.AgentID
	headers["AgentName"] = DATASTACKAgent.AgentName
	headers["AgentType"] = DATASTACKAgent.AgentType
	headers["DATA-STACK-App-Name"] = DATASTACKAgent.AppName
	headers["DATA-STACK-Agent-Paused"] = strconv.FormatBool(DATASTACKAgent.Paused)
	request := models.GetRunningOrPendingFlowsFromIMRequest{}
	request.VaultVersion = DATASTACKAgent.VaultVersion
	request.Release = DATASTACKAgent.Release

	err := DATASTACKAgent.Utils.MakeJSONRequest(client, DATASTACKAgent.BaseURL+"/getrunningorpendingflowsfrompartnermanager", request, headers, &data)
	if err != nil {
		DATASTACKAgent.addEntryToTransferLedger("", "", "", "", ledgers.AGENTRESTARTERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		if strings.Contains(messagegenerator.ExtractErrorMessageFromErrorObject(err), messagegenerator.InvalidIP) {
			DATASTACKAgent.Logger.Error("IP is not trusted, stopping the agent")
		} else {
			DATASTACKAgent.Logger.Error(fmt.Sprintf("%s", err))
		}
		os.Exit(0)
	}
	if !DATASTACKAgent.Paused {
		switch data.Status {
		case AlreadyRunningAgent:
			DATASTACKAgent.Logger.Error(messagegenerator.AgentAlreadyRunningError)
			os.Exit(0)
		case DisabledAgent:
			DATASTACKAgent.Logger.Error(messagegenerator.AgentDisabledError)
			os.Exit(0)
		case VaultVersionChanged:
			DATASTACKAgent.Logger.Error(messagegenerator.VaultVersionChangedError)
			os.Exit(0)
		case ReleaseChanged:
			DATASTACKAgent.Logger.Error(messagegenerator.ReleaseVersionChangedError)
			os.Exit(0)
		}
	}

	if data.Mode != "" {
		DATASTACKAgent.updateAgentModeAndPortNumber(data.Mode)
	}

	DATASTACKAgent.Logger.Info("%v flows fetched ", len(data.TransferLedgerEntries))
	for _, entry := range data.TransferLedgerEntries {
		switch entry.Action {
		case ledgers.FLOWCREATEREQUEST:
			DATASTACKAgent.handleFlowCreateStartOrUpdateRequest(entry)
		case ledgers.FLOWSTARTREQUEST:
			DATASTACKAgent.handleFlowCreateStartOrUpdateRequest(entry)
		}
	}
}

//InitCentralHeartBeat - heartbeat for agents
func (DATASTACKAgent *AgentDetails) initCentralHeartBeat(wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	var client = DATASTACKAgent.fetchHTTPClient()
	frequency, err := strconv.Atoi(DATASTACKAgent.HeartBeatFrequency)
	if err != nil {
		frequency = 10
	}
	for {
		if !DATASTACKAgent.Paused {
			monitoringLedgerEntries, err := DATASTACKAgent.MonitoringLedger.GetAllEntries()
			if err != nil {
				DATASTACKAgent.addEntryToTransferLedger("", "", "", "", ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
				continue
			}
			transferLedgerEntries, err := DATASTACKAgent.TransferLedger.GetUnsentNotifications()
			if err != nil {
				DATASTACKAgent.addEntryToTransferLedger("", "", "", "", ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
				continue
			}
			request := models.CentralHeartBeatRequest{
				MonitoringLedgerEntries: monitoringLedgerEntries,
				TransferLedgerEntries:   transferLedgerEntries,
			}

			request.MonitoringLedgerEntries[0].Release = DATASTACKAgent.Release
			data := models.CentralHeartBeatResponse{}
			headers := make(map[string]string)
			headers["AgentID"] = DATASTACKAgent.AgentID
			headers["AgentVersion"] = DATASTACKAgent.AgentVersion
			headers["AgentName"] = DATASTACKAgent.AgentName
			headers["NodeHeartBeatFrequency"] = DATASTACKAgent.HeartBeatFrequency
			headers["AgentType"] = DATASTACKAgent.AgentType
			headers["DATA-STACK-App-Name"] = DATASTACKAgent.AppName
			DATASTACKAgent.Logger.Trace("Heartbeat Headers %v", headers)
			DATASTACKAgent.Logger.Info("Making Request at -: %s", DATASTACKAgent.BaseURL+"/heartbeat")
			if !DATASTACKAgent.Paused {
				err = DATASTACKAgent.Utils.MakeJSONRequest(client, DATASTACKAgent.BaseURL+"/heartbeat", request, headers, &data)
			} else {
				continue
			}

			if err != nil {
				DATASTACKAgent.addEntryToTransferLedger("", "", "", "", ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
				if strings.Contains(messagegenerator.ExtractErrorMessageFromErrorObject(err), messagegenerator.InvalidIP) {
					DATASTACKAgent.Logger.Error("IP is not trusted, stopping the agent")
					os.Exit(0)
				}
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
				} else {
					DATASTACKAgent.Vault.Upsert(maxconcurrentuploads, data.AgentMaxConcurrentUploads)
				}
			}

			for _, entry := range transferLedgerEntries {
				err = DATASTACKAgent.TransferLedger.UpdateSentOrReadFieldOfEntry(&entry, true)
				if err != nil {
					DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.HEARTBEATERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
					continue
				}
			}

			transferLedgerEntries = nil
			monitoringLedgerEntries = nil
			time.Sleep(time.Duration(frequency) * time.Second)
		}

	}
}

//FetchAgentDetailsFromVault - fetch agent details from vault
func (DATASTACKAgent *AgentDetails) fetchAgentDetailsFromVault() error {
	var err error
	key, err := DATASTACKAgent.Vault.Get("datastack.key")
	if err != nil {
		return err
	}
	keyString := string(key)
	DATASTACKAgent.DATASTACKKeyFile, err = base64.StdEncoding.DecodeString(keyString)
	if err != nil {
		return err
	}
	cert, err := DATASTACKAgent.Vault.Get("datastack.cert")
	if err != nil {
		return err
	}
	certString := string(cert)
	DATASTACKAgent.DATASTACKCertFile, err = base64.StdEncoding.DecodeString(certString)
	if err != nil {
		return err
	}
	caCertString, err := DATASTACKAgent.Vault.Get("trustCerts")
	if err != nil {
		return err
	}
	var trustStore []string
	if len(caCertString) > 0 {
		err := json.Unmarshal(caCertString, &trustStore)
		if err != nil {
			DATASTACKAgent.Logger.Error("Error in fetching trust store for agent")
			os.Exit(0)
		}
	}
	DATASTACKAgent.TrustCerts = trustStore
	DATASTACKAgent.TrustCerts = append(DATASTACKAgent.TrustCerts, certString)
	agentID, err := DATASTACKAgent.Vault.Get("agent-id")
	if err != nil {
		return err
	}
	agentName, err := DATASTACKAgent.Vault.Get("agent-name")
	if err != nil {
		return err
	}
	appName, err := DATASTACKAgent.Vault.Get("app-name")
	if err != nil {
		return err
	}
	agentVersion, err := DATASTACKAgent.Vault.Get("agent-version")
	if err != nil {
		return err
	}
	uploadRetryCounter, err := DATASTACKAgent.Vault.Get("upload-retry-counter")
	if err != nil {
		return err
	}
	downloadRetryCounter, err := DATASTACKAgent.Vault.Get("download-retry-counter")
	if err != nil {
		return err
	}
	baseURL, err := DATASTACKAgent.Vault.Get("base-url")
	if err != nil {
		return err
	}
	mode, err := DATASTACKAgent.Vault.Get("mode")
	if err != nil {
		return err
	}
	encryptFile, err := DATASTACKAgent.Vault.Get("encrypt-file")
	if err != nil {
		return err
	}
	retainFileOnSuccess, err := DATASTACKAgent.Vault.Get("retain-file-on-success")
	if err != nil {
		return err
	}
	retainFileOnError, err := DATASTACKAgent.Vault.Get("retain-file-on-error")
	if err != nil {
		return err
	}
	maxConcurrentUploads, err := DATASTACKAgent.Vault.Get("max-concurrent-uploads")
	if err != nil {
		return err
	}
	sentinelMaxMissesCount, err := DATASTACKAgent.Vault.Get("sentinel-max-misses-count")
	if err != nil {
		return err
	}
	vaultVersion, err := DATASTACKAgent.Vault.Get("vault-version")
	if err != nil {
		return err
	}
	release, err := DATASTACKAgent.Vault.Get("release")
	if err != nil {
		return err
	}
	maxLogBackUpIndex, err := DATASTACKAgent.Vault.Get("b2b_agent_log_retention_count")
	if err != nil {
		return err
	}
	logRotationType, err := DATASTACKAgent.Vault.Get("b2b_agent_log_rotation_type")
	if err != nil {
		return err
	}
	maxFileSize, err := DATASTACKAgent.Vault.Get("b2b_agent_log_max_file_size")
	if err != nil {
		return err
	}

	DATASTACKAgent.AgentID = string(agentID)
	DATASTACKAgent.AgentName = string(agentName)
	DATASTACKAgent.AppName = string(appName)
	DATASTACKAgent.AgentVersion = string(agentVersion)
	DATASTACKAgent.UploadRetryCounter = string(uploadRetryCounter)
	DATASTACKAgent.BaseURL = string(baseURL)
	DATASTACKAgent.Mode = string(mode)
	DATASTACKAgent.DownloadRetryCounter = string(downloadRetryCounter)
	DATASTACKAgent.SentinelMaxMissesCount = string(sentinelMaxMissesCount)
	DATASTACKAgent.VaultVersion, err = strconv.Atoi(string(vaultVersion))
	if err != nil {
		return err
	}
	DATASTACKAgent.Release = string(release)
	if strings.ToUpper(string(mode)) == "PROD" {
		DATASTACKAgent.LogRotationType = string(logRotationType)
		DATASTACKAgent.MaxLogBackUpIndex = string(maxLogBackUpIndex)
		DATASTACKAgent.LogMaxFileSize = string(maxFileSize)
	}

	i := 5
	if string(maxConcurrentUploads) != "" {
		i, err = strconv.Atoi(string(maxConcurrentUploads))
		if err != nil {
			i = 5
		}
	}

	DATASTACKAgent.EncryptFile = "true"
	DATASTACKAgent.RetainFileOnSuccess = "true"
	DATASTACKAgent.RetainFileOnError = "true"
	if string(encryptFile) != "" {
		DATASTACKAgent.EncryptFile = string(encryptFile)
	}

	if string(retainFileOnSuccess) != "" {
		DATASTACKAgent.RetainFileOnSuccess = string(retainFileOnSuccess)
	}

	if string(retainFileOnError) != "" {
		DATASTACKAgent.RetainFileOnError = string(retainFileOnError)
	}
	DATASTACKAgent.MaxConcurrentUploads = i
	DATASTACKAgent.MaxConcurrentDownloads = 15
	return nil
}

//InitAgentServer start an server from agent side
func (DATASTACKAgent *AgentDetails) initAgentServer() error {
	r := mux.NewRouter()
	r.HandleFunc("/encryptFile", DATASTACKAgent.encryptFileHandler)
	r.HandleFunc("/decryptFile", DATASTACKAgent.decryptFileHandler)
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

//EncryptFileHandler - for encrypting the input file by custom input password
func (DATASTACKAgent *AgentDetails) encryptFileHandler(w http.ResponseWriter, r *http.Request) {
	data := models.EncryptionDecryptionTool{}
	buffer, err := DATASTACKAgent.Utils.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(fmt.Sprintf("%s", err)))
		return
	}

	err = json.Unmarshal(buffer, &data)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(fmt.Sprintf("%s", err)))
		return
	}
	err = DATASTACKAgent.encryptFileHandlerLogic(data)
	if err != nil {
		response := models.EncryptionDecryptionToolMessage{}
		response.Message = fmt.Sprintf("%s", err)
		responseData, _ := json.Marshal(&response)
		w.WriteHeader(200)
		w.Write(responseData)
	} else {
		response := models.EncryptionDecryptionToolMessage{}
		response.Message = "File successfully encrypted"
		responseData, _ := json.Marshal(&response)
		w.WriteHeader(200)
		w.Write(responseData)
	}
}

//EncryptFileHandlerLogic - command line tool logic for encrypting the file
func (DATASTACKAgent *AgentDetails) encryptFileHandlerLogic(data models.EncryptionDecryptionTool) error {
	data.InputFilePath = returnAbsolutePath(data.InputFilePath)
	data.OutputFilePath = returnAbsolutePath(data.OutputFilePath)
	if data.InputFilePath == data.OutputFilePath {
		return errors.New("Error -:" + messagegenerator.EncryptDecryptFileHandlerError)
	}
	err := DATASTACKAgent.Utils.EncryptFileInChunksAndWriteInOutputFile(data.InputFilePath, data.OutputFilePath, data.Password, MaxChunkSize)
	if err != nil {
		os.Remove(data.OutputFilePath)
		return err
	} else {
		decryptedFileChecksum, err := DATASTACKAgent.Utils.CalculateMD5ChecksumForFile(data.InputFilePath)
		if err != nil {
			return err
		}
		encryptedFileChecksum, err := DATASTACKAgent.Utils.CalculateMD5ChecksumForFile(data.OutputFilePath)
		if err != nil {
			return err
		}
		fileStat, _ := os.Stat(data.InputFilePath)
		fileSize := fileStat.Size()
		totalChunks := math.Ceil(float64(fileSize) / float64(MaxChunkSize))
		vaultValue := data.Password + ":" + decryptedFileChecksum + ":" + strconv.FormatInt(int64(totalChunks), 10)
		DATASTACKAgent.Vault.Upsert(encryptedFileChecksum, vaultValue)
	}
	return nil
}

//DecryptFileHandler -  for decrypting the encrypted file
func (DATASTACKAgent *AgentDetails) decryptFileHandler(w http.ResponseWriter, r *http.Request) {
	data := models.EncryptionDecryptionTool{}
	buffer, err := DATASTACKAgent.Utils.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(fmt.Sprintf("%s", err)))
		return
	}

	err = json.Unmarshal(buffer, &data)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(fmt.Sprintf("%s", err)))
		return
	}

	err = DATASTACKAgent.decryptFileHandlerLogic(data)
	response := models.EncryptionDecryptionToolMessage{}
	if err != nil {
		response.Message = fmt.Sprintf("%s", err)
		responseData, _ := json.Marshal(&response)
		w.WriteHeader(200)
		w.Write(responseData)
	} else {
		response.Message = "File successfully decrypted"
		responseData, _ := json.Marshal(&response)
		w.WriteHeader(200)
		w.Write(responseData)
	}
}

//DecryptFileHandlerLogic -  logic for decrypting the input file using command line
func (DATASTACKAgent *AgentDetails) decryptFileHandlerLogic(data models.EncryptionDecryptionTool) error {
	data.InputFilePath = returnAbsolutePath(data.InputFilePath)
	data.OutputFilePath = returnAbsolutePath(data.OutputFilePath)
	if data.InputFilePath == data.OutputFilePath {
		return errors.New("Error -:" + messagegenerator.EncryptDecryptFileHandlerError)
	}
	err := DATASTACKAgent.Utils.DecryptFileInChunksAndWriteInOutputFile(data.InputFilePath, data.OutputFilePath, data.Password, BytesToSkipWhileDecrypting)
	if err != nil {
		os.Remove(data.OutputFilePath)
	}
	return err
}

//FetchHTTPClient -  getting http client with all certs
func (DATASTACKAgent *AgentDetails) fetchHTTPClient() *http.Client {
	var client *http.Client
	if len(DATASTACKAgent.DATASTACKCertFile) != 0 && len(DATASTACKAgent.DATASTACKKeyFile) != 0 {
		client = DATASTACKAgent.Utils.GetNewHTTPClient(DATASTACKAgent.Utils.PrepareTLSTransportConfigWithEncodedTrustStore(DATASTACKAgent.DATASTACKCertFile, DATASTACKAgent.DATASTACKKeyFile, DATASTACKAgent.TrustCerts))
	} else {
		client = DATASTACKAgent.Utils.GetNewHTTPClient(nil)
	}
	return client
}

func (DATASTACKAgent *AgentDetails) addEntryToTransferLedger(namespace string, flowName string, flowID string, deploymentName string, action string, metaData string, time time.Time, entryType string, sentOrRead bool) error {
	return DATASTACKAgent.TransferLedger.AddEntry(&models.TransferLedgerEntry{
		AgentID:        DATASTACKAgent.AgentID,
		AgentName:      DATASTACKAgent.AgentName,
		AppName:        DATASTACKAgent.AppName,
		Namespace:      namespace,
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

//updateAgentModeAndPortNumber - update agent mode and port number
func (DATASTACKAgent *AgentDetails) updateAgentModeAndPortNumber(mode string) error {
	err := DATASTACKAgent.Vault.Upsert("mode", mode)
	if err != nil {
		return err
	}
	if mode == "PROD" {
		portNumberForProd, _ := DATASTACKAgent.Vault.Get("agent-port-number")
		if string(portNumberForProd) != "" {
			DATASTACKAgent.AgentPortNumber = string(portNumberForProd)
			return DATASTACKAgent.updateAgentConfFile()
		}
	}
	return nil
}

func (DATASTACKAgent *AgentDetails) updateAgentConfFile() error {
	data := []string{}
	data = append(data, "central-folder="+DATASTACKAgent.CentralFolder)
	data = append(data, "sentinal-port-number="+DATASTACKAgent.SentinelPortNumber)
	data = append(data, "agent-port-number="+DATASTACKAgent.AgentPortNumber)
	data = append(data, "heartbeat-frequency="+DATASTACKAgent.HeartBeatFrequency)
	data = append(data, "log-level="+DATASTACKAgent.LogLevel)
	err := DATASTACKAgent.Utils.WriteLinesToFile(data, DATASTACKAgent.ConfFilePath)
	if err != nil {
		DATASTACKAgent.Logger.Error("%s", err)
		return err
	}
	return nil
}

func (DATASTACKAgent *AgentDetails) processQueuedDownloads() {
	DATASTACKAgent.Logger.Info("Started Queued Downloads")
	for {
		var err error
		transferLedgerEntries := []models.TransferLedgerEntry{}

		if !DATASTACKAgent.Paused {
			transferLedgerEntries, err = DATASTACKAgent.TransferLedger.GetFileDownloadRequests(DATASTACKAgent.MaxConcurrentDownloads)
			if err != nil {
				DATASTACKAgent.addEntryToTransferLedger("", "", "", "", ledgers.QUEUEDJOBSERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
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
					DATASTACKAgent.addEntryToTransferLedger("", "", "", "", ledgers.QUEUEDJOBSERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
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
		DATASTACKAgent.Logger.Error("File download error during json unmarshalling %s ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
		blockOpener <- true
		return
	}

	if DATASTACKAgent.TransferLedger.IsFileAlreadyDownloaded(fileDownloadMetaData.FileID) {
		blockOpener <- true
		return
	}

	DATASTACKAgent.Logger.Info("Started file download for Flow-: %v, FlowId-: %v", entry.FlowName, entry.FlowID)
	DATASTACKAgent.Logger.Debug("Entry details %s", entry.MetaData)

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
			DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.OUTPUTDIRECTORYDOESNOTEXISTERROR, metadatagenerator.GenerateFileDownloadErrorMetaData("Output Directory Doesn't Exist , Create Output Directory And Restart the Agent", fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
			blockOpener <- true
			return

		} else {
			file, err := os.Create(path + string(os.PathSeparator) + fileDownloadMetaData.FileName)
			if err != nil {
				DATASTACKAgent.Logger.Error("File download error 1 - %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
				DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
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
				DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.OUTPUTDIRECTORYDOESNOTEXISTERROR, metadatagenerator.GenerateFileDownloadErrorMetaData("Output Directory Doesn't Exist , Create Output Directory And Restart the Agent", fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
				continue
			} else {
				file, err := os.Create(path + string(os.PathSeparator) + fileDownloadMetaData.FileName)
				if err != nil {
					DATASTACKAgent.Logger.Error("File download error 2 - %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
					DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
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
			DATASTACKAgent.Logger.Error("File download error 3 - %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
			DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
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
		AgentName:      DATASTACKAgent.AgentName,
		AgentID:        DATASTACKAgent.AgentID,
		AgentVersion:   DATASTACKAgent.AgentVersion,
		FileName:       fileDownloadMetaData.FileName,
		FlowName:       entry.FlowName,
		FlowID:         entry.FlowID,
		AppName:        entry.AppName,
		FileID:         fileDownloadMetaData.FileID,
		DeploymentName: entry.DeploymentName,
	}

	payload, err := json.Marshal(data)
	if err != nil {
		DATASTACKAgent.Logger.Error("File download error during payload marshalling - %s ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
		removeFiles(files)
		blockOpener <- true
		return
	}

	var downloadFileError error
	var dataToWriteInFile []byte
	checksumsToVerify := strings.Split(fileDownloadMetaData.ChunkChecksumList, ",")
	currentChunk := 1
	TotalChunks, _ := strconv.Atoi(fileDownloadMetaData.TotalChunks)

	if TotalChunks > 1 {
		DATASTACKAgent.Logger.Debug("File size is greater than 100MB, agent will download the file in chunks.")
	}

	for i := 0; i < retryCount; i++ {
		for currentChunk <= TotalChunks {
			DATASTACKAgent.Logger.Info("Downloading chunk %v/%v of %s for flow", currentChunk, TotalChunks, fileDownloadMetaData.FileName, entry.FlowName)
			client = DATASTACKAgent.fetchHTTPClient()
			url := DATASTACKAgent.BaseURL + "/download"
			req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("file-id", data.FileID)
			req.Header.Set("AgentType", DATASTACKAgent.AgentType)
			req.Header.Set("DATA-STACK-App-Name", DATASTACKAgent.AppName)
			req.Header.Set("AgentID", DATASTACKAgent.AgentID)
			req.Header.Set("DATA-STACK-Chunk-Number", strconv.Itoa(currentChunk))
			req.Header.Set("DATA-STACK-BufferEncryption", "true")
			DATASTACKAgent.updateHeaders(&req.Header)
			if err != nil {
				downloadFileError = err
				DATASTACKAgent.Logger.Error("File download error 4 - %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
				DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
				break
			}
			req.Close = true
			response, err := client.Do(req)
			if err != nil {
				downloadFileError = err
				DATASTACKAgent.Logger.Error("File download error while requesting download api %s %s %s", entry.FlowName, fileDownloadMetaData.FileName, messagegenerator.ExtractErrorMessageFromErrorObject(err))
				DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
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
				chunkChecksum := DATASTACKAgent.Utils.CalculateMD5ChecksumForByteSlice(encryptedChunk)
				if checksumsToVerify[currentChunk-1] != chunkChecksum {
					downloadFileError = errors.New("Chunk Checksum match failed for download file - " + fileDownloadMetaData.FileName)
					DATASTACKAgent.Logger.Error("Checksum match failed for download file - " + fileDownloadMetaData.FileName)
					encryptedChunk = nil
					DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, metadatagenerator.GenerateErrorMessageMetaData("Checksum match failed for downloaded file - "+fileDownloadMetaData.FileName), time.Now(), "OUT", false)
					break
				} else if DATASTACKAgent.EncryptFile == "true" {
					dataToWriteInFile = encryptedChunk
					encryptedChunk = nil
				} else {
					compressedChunk, err := DATASTACKAgent.Utils.DecryptData(encryptedChunk[64:], fileDownloadMetaData.Password)
					dataToWriteInFile = DATASTACKAgent.Utils.Decompress(compressedChunk)
					if err != nil {
						downloadFileError = err
						DATASTACKAgent.Logger.Error("Decrypting chunk error -: ", err)
						encryptedChunk = nil
						compressedChunk = nil
						break
					}
					compressedChunk = nil
				}

				for i := 0; i < len(files); i++ {
					_, err = files[i].Write(dataToWriteInFile)
					if err != nil {
						DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
						DATASTACKAgent.Logger.Error("Writing download file error ", err)
						removeFiles(files)
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
					removeFiles(files)
					files = nil
					os.Exit(0)
				}

				if err != nil {
					DATASTACKAgent.Logger.Error(messagegenerator.ExtractErrorMessageFromErrorObject(err))
					DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
				} else {
					DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, string(resp), time.Now(), "OUT", false)
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
				DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.REDOWNLOADED, string(interactionMetadataString), time.Now(), "OUT", false)
			} else {
				DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADED, string(interactionMetadataString), time.Now(), "OUT", false)
				if fileDownloadMetaData.SuccessBlock == "true" {
					go DATASTACKAgent.handleSuccessFlowFileUploadRequest(entry)
				}
			}
			DATASTACKAgent.Logger.Info("%s file downloaded successfully", fileDownloadMetaData.FileName)
			DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADED+fileDownloadMetaData.FileID, "DOWNLOADED"+fileDownloadMetaData.FileID, time.Now(), "OUT", true)
			closeFiles(files)
			files = nil
			blockOpener <- true
			return
		}
	}
	closeFiles(files)
	DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.DOWNLOADERROR, metadatagenerator.GenerateFileDownloadErrorMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(downloadFileError), fileDownloadMetaData.RemoteTxnID, fileDownloadMetaData.DataStackTxnID), time.Now(), "OUT", false)
	blockOpener <- true
}

//removeFiles -: deleting list of files
func removeFiles(files []*os.File) {
	for i := 0; i < len(files); i++ {
		if files[i] != nil {
			os.Remove(files[i].Name())
		}
	}
}

//closeFiles - closing files
func closeFiles(files []*os.File) {
	for i := 0; i < len(files); i++ {
		if files[i] != nil {
			files[i].Close()
		}
	}
}

func (DATASTACKAgent *AgentDetails) updateHeaders(headers *http.Header) {
	headers.Set("AgentID", DATASTACKAgent.AgentID)
	headers.Set("AgentVersion", DATASTACKAgent.AgentVersion)
	headers.Set("AgentName", DATASTACKAgent.AgentName)
	headers.Set("NodeHeartBeatFrequency", DATASTACKAgent.HeartBeatFrequency)
}

func (DATASTACKAgent *AgentDetails) handleSuccessFlowFileUploadRequest(entry models.TransferLedgerEntry) {
	DATASTACKAgent.Logger.Debug("Handling Success Flow File Upload Request %s", entry.FlowName)
	DATASTACKAgent.Logger.Debug("Entry details -: %v", entry)
	DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.UPLOADFILETOSUCCESSFLOW, "", time.Now(), "OUT", false)

	fileDownloadMetaData := models.DownloadFileRequestMetaData{}
	err := json.Unmarshal([]byte(entry.MetaData), &fileDownloadMetaData)
	if err != nil {
		DATASTACKAgent.Logger.Error("Success Flow metadata Unmarshalling Error %s ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.UPLOADFILETOSUCCESSFLOWERROR, "", time.Now(), "OUT", false)
		return
	}

	data := models.DownloadFileRequest{
		AgentName:      DATASTACKAgent.AgentName,
		AgentID:        DATASTACKAgent.AgentID,
		AgentVersion:   DATASTACKAgent.AgentVersion,
		FileName:       fileDownloadMetaData.FileName,
		FlowName:       entry.FlowName,
		FlowID:         entry.FlowID,
		AppName:        entry.AppName,
		FileID:         fileDownloadMetaData.FileID,
		DeploymentName: entry.DeploymentName,
	}
	payload, err := json.Marshal(data)
	if err != nil {
		DATASTACKAgent.Logger.Error("Success Flow Upload Marshalling Error %s ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.UPLOADFILETOSUCCESSFLOWERROR, "", time.Now(), "OUT", false)
		return
	}

	var client = DATASTACKAgent.fetchHTTPClient()
	req, _ := http.NewRequest("POST", DATASTACKAgent.BaseURL+"/successFlowUpload", bytes.NewReader(payload))
	DATASTACKAgent.updateHeaders(&req.Header)
	req.Header.Set("AgentType", DATASTACKAgent.AgentType)
	req.Header.Set("AgentID", DATASTACKAgent.AgentID)
	req.Header.Set("DATA-STACK-App-Name", DATASTACKAgent.AppName)
	req.Header.Set("DATA-STACK-Flow-Name", entry.FlowName)
	req.Header.Set("DATA-STACK-Flow-Id", entry.FlowID)
	req.Header.Set("DATA-STACK-Remote-Txn-Id", fileDownloadMetaData.RemoteTxnID)
	req.Header.Set("DATA-STACK-Txn-Id", fileDownloadMetaData.DataStackTxnID)
	req.Header.Set("DATA-STACK-Deployment-Name", entry.DeploymentName)
	req.Header.Set("namespace", entry.Namespace)
	req.Header.Set("fileID", fileDownloadMetaData.FileID)
	req.Header.Set("DATA-STACK-Operating-System", runtime.GOOS)
	req.Header.Set("filePassword", fileDownloadMetaData.Password)
	DATASTACKAgent.Logger.Info("Making request at %s", DATASTACKAgent.BaseURL+"/successFlowUpload")
	DATASTACKAgent.Logger.Debug("Request Headers %v", req.Header)
	req.Close = true
	_, err = client.Do(req)
	if err != nil {
		DATASTACKAgent.Logger.Error("Success Flow File Upload Error %s ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.UPLOADFILETOSUCCESSFLOWERROR, "", time.Now(), "OUT", false)
	}
}

func returnAbsolutePath(inputDirectory string) string {
	inputDirectory = strings.Replace(inputDirectory, "\\", "\\\\", -1)
	absoluteInputPath, _ := filepath.Abs(inputDirectory)
	if absoluteInputPath[0] == '\\' && absoluteInputPath[1] != '\\' {
		absoluteInputPath = string(os.PathSeparator) + absoluteInputPath
	}
	return absoluteInputPath
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
		DATASTACKAgent.getRunningOrPendingFlowFromIntegrationManagerAfterRestart()
		DATASTACKAgent.Paused = false
		DATASTACKAgent.Logger.Info("Agent resumes, all operations started again.")
	})
	DATASTACKAgent.Logger.Info("Starting CompactDB Handler ...")
	c.Start()
}

func (DATASTACKAgent *AgentDetails) handleFlowCreateStartOrUpdateRequest(entry models.TransferLedgerEntry) {
	switch entry.Action {
	case ledgers.FLOWCREATEREQUEST:
		DATASTACKAgent.Logger.Info("Handling Flow Creation Request %s", entry.FlowName)
		DATASTACKAgent.Logger.Debug("Entry details -: %v", entry)
		if DATASTACKAgent.Flows[entry.AppName] != nil && DATASTACKAgent.Flows[entry.AppName][entry.FlowID] != nil {
			DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWALREADYSTARTED, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.FlowAlreadyRunningError), time.Now(), "OUT", false)
			return
		}
	case ledgers.FLOWSTARTREQUEST:
		DATASTACKAgent.Logger.Info("Handling Flow Start Request %s", entry.FlowName)
		DATASTACKAgent.Logger.Debug("Entry details -: %v", entry)
		if DATASTACKAgent.Flows[entry.AppName] != nil && DATASTACKAgent.Flows[entry.AppName][entry.FlowID] != nil {
			DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWALREADYSTARTED, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.FlowAlreadyRunningError), time.Now(), "OUT", false)
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
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWCREATIONERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
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
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWCONFWRITEERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		return
	}
	values, err := DATASTACKAgent.Utils.ReadFlowConfigurationFile(flowFolder + string(os.PathSeparator) + "flow.conf")
	if err != nil {
		DATASTACKAgent.Logger.Error("Read Flow conf file error %s", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWCONFREADERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		return
	}
	DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWCREATED, entry.MetaData, time.Now(), "OUT", false)

	values["parent-directory"] = flowFolder

	if !listener && newFlowReq.Mirror {
		DATASTACKAgent.Logger.Debug("Fetching Directory Structure For Mirroring %s", string(entry.MetaData))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.GETMIRRORFOLDERSTRUCTURE, entry.MetaData, time.Now(), "OUT", false)
	}

	if listener && newFlowReq.Mirror {
		DATASTACKAgent.Logger.Debug("Sending Update Request to receive the Input Directory Structure for Mirroring")
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
	properties.DeploymentName = entry.DeploymentName
	properties.Namespace = entry.Namespace
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
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWSTARTED, entry.MetaData, time.Now(), "OUT", false)
		DATASTACKAgent.Logger.Info("%s flow create request completed", entry.FlowName)
	case ledgers.FLOWSTARTREQUEST:
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWSTARTED, entry.MetaData, time.Now(), "OUT", false)
		DATASTACKAgent.Logger.Info("%s flow start request completed", entry.FlowName)
	case ledgers.FLOWUPDATEREQUEST:
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWUPDATED, entry.MetaData, time.Now(), "OUT", false)
		DATASTACKAgent.Logger.Info("%s flow update request completed", entry.FlowName)
	}
}

func (DATASTACKAgent *AgentDetails) handleFlowStopRequest(entry models.TransferLedgerEntry) {
	DATASTACKAgent.Logger.Info("Handling Flow Stop Request %s", entry.FlowName)
	DATASTACKAgent.Logger.Debug("entry details -: %v", entry)
	flowID := entry.FlowID
	if DATASTACKAgent.Flows[entry.AppName] == nil || DATASTACKAgent.Flows[entry.AppName][flowID] == nil {
		DATASTACKAgent.Logger.Error("%s %s", entry.FlowName, messagegenerator.NoFlowsExistError)
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWSTOPERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.NoFlowsExistError), time.Now(), "OUT", false)
		return
	}
	data, err := DATASTACKAgent.Utils.ReadFlowConfigurationFile(DATASTACKAgent.Flows[entry.AppName][flowID]["parent-directory"] + string(os.PathSeparator) + "flow.conf")
	if err != nil {
		DATASTACKAgent.Logger.Error("Read flow conf error %s", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWSTOPERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
		return
	}
	err = DATASTACKAgent.Utils.CreateOrUpdateFlowConfFile(DATASTACKAgent.Flows[entry.AppName][flowID]["parent-directory"]+string(os.PathSeparator)+"flow.conf", &models.FlowDefinitionResponse{
		FlowName:   entry.FlowName,
		FileSuffix: data["file-suffix"],
		FlowID:     entry.FlowID,
	}, false, entry.DeploymentName)
	if err != nil {
		DATASTACKAgent.Logger.Error("Update flow conf error %s", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWSTOPERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
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
	DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWSTOPPED, entry.MetaData, time.Now(), "OUT", false)
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

func (DATASTACKAgent *AgentDetails) handleGetMirrorFolderStructure(entry models.TransferLedgerEntry) {
	DATASTACKAgent.Logger.Debug("Handling Get Mirror Folder Structure Request - %v", entry)
	var mirrorInputFolderPaths []string
	flowDef := models.FlowDefinitionResponse{}
	err := json.Unmarshal([]byte(entry.MetaData), &flowDef)
	if err != nil {
		DATASTACKAgent.Logger.Error(messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.GETMIRRORFOLDERSTRUCTUREERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
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
	DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.RECEIVEMIRRORFOLDERSTRUCTURE, entry.MetaData, time.Now(), "OUT", false)
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
		DATASTACKAgent.Logger.Debug("Listener started on %s ", properties.InputFolder)
		DATASTACKAgent.initExistingUploadsFromInputFolder(properties.InputFolder, entry, properties.BlockName, properties.StructureID, properties.UniqueRemoteTxn, properties.UniqueRemoteTxnOptions, pollingUploads, properties.ErrorBlocks)
		DATASTACKAgent.Logger.Debug("Listener stopped on %s ", properties.InputFolder)
	})
	c.Start()
	end := <-DATASTACKAgent.FlowsChannel[properties.AppName][properties.FlowID]
	c.Stop()
	DATASTACKAgent.Logger.Info("%s on directory %s for agent %s and flow %s", end, properties.InputFolder, properties.AgentName, properties.FlowName)
	deAllocateInputDirectory(properties.InputFolder)
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
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.FLOWCREATIONERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
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
	properties.DeploymentName = entry.DeploymentName
	properties.Namespace = entry.Namespace
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
			DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.PREPROCESSINGFILEERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
			return
		}
		inputFiles = append(inputFiles, file)
	} else {
		inputFiles, err = ioutil.ReadDir(inputFolder)
		if err != nil {
			DATASTACKAgent.Logger.Error("Error in init existing uploads - ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
			DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.PREPROCESSINGFILEERROR, metadatagenerator.GenerateErrorMessageMetaData(messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
			return
		}
	}

	for _, inputFile := range inputFiles {
		if !inputFile.IsDir() && inputFile.Name() != "OSLevelTrapCheck" {
			_, err := regexp.MatchString(values["file-suffix"], inputFile.Name())
			if err != nil {
				DATASTACKAgent.Logger.Error(fmt.Sprintf("%s", err))
			}
			matched := utils.CheckFileRegexAndExtension(inputFile.Name(), newFlowReq.FileExtensions, newFlowReq.FileNameRegexs)
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
					DATASTACKAgent.addEntryToTransferLedger(properties.Namespace, properties.FlowName, properties.FlowID, properties.DeploymentName, ledgers.FILEUPLOADERRORDUETOLARGEFILENAMEORMAXFILESIZE, metadatagenerator.GenerateFileUploadErrorMetaDataForLargeFileNameOrMaxFileSize(originalFileName, originalAbsoluteFilePath, mirrorPath, properties.UniqueRemoteTxn, properties.UniqueRemoteTxnOptions.FileName, properties.UniqueRemoteTxnOptions.Checksum, properties.StructureID, properties.BlockName, properties.InputFolder, newFileName, newLocation, md5CheckSum, remoteTxnID, dataStackTxnID, messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
					os.Rename(inputFolder+string(os.PathSeparator)+inputFile.Name(), newLocation+string(os.PathSeparator)+newFileName)
					DATASTACKAgent.Utils.CreateFlowErrorFile(newLocation+string(os.PathSeparator)+dataStackTxnID+"_"+originalFileName+".Error.txt", messagegenerator.ExtractErrorMessageFromErrorObject(err))
					DATASTACKAgent.Logger.Error("Moving file to error folder - " + messagegenerator.ExtractErrorMessageFromErrorObject(err))
					if errorBlocks {
						go DATASTACKAgent.handleErrorFlowRequest(entry, properties, remoteTxnID, dataStackTxnID, messagegenerator.ExtractErrorMessageFromErrorObject(err))
					}
					continue
				} else if err != nil {
					remoteTxnID := originalFileName
					u := uuid.NewV4()
					dataStackTxnID := u.String()
					flowFolder := DATASTACKAgent.AppFolderPath + string(os.PathSeparator) + strings.Replace(properties.FlowName, " ", "_", -1)
					newFileName = time.Now().Format("2006_01_02_15_04_05_000000") + "____" + originalFileName + ".error"
					newLocation = flowFolder + string(os.PathSeparator) + "error"
					DATASTACKAgent.addEntryToTransferLedger(properties.Namespace, properties.FlowName, properties.FlowID, properties.DeploymentName, ledgers.WATCHERERROR, metadatagenerator.GenerateFileUploadErrorMetaDataForLargeFileNameOrMaxFileSize(originalFileName, originalAbsoluteFilePath, mirrorPath, properties.UniqueRemoteTxn, properties.UniqueRemoteTxnOptions.FileName, properties.UniqueRemoteTxnOptions.Checksum, properties.StructureID, properties.BlockName, properties.InputFolder, newFileName, newLocation, md5CheckSum, remoteTxnID, dataStackTxnID, messagegenerator.ExtractErrorMessageFromErrorObject(err)), time.Now(), "OUT", false)
					os.Rename(inputFolder+string(os.PathSeparator)+inputFile.Name(), newLocation+string(os.PathSeparator)+newFileName)
					DATASTACKAgent.Utils.CreateFlowErrorFile(newLocation+string(os.PathSeparator)+dataStackTxnID+"_"+originalFileName+".Error.txt", messagegenerator.ExtractErrorMessageFromErrorObject(err))
					DATASTACKAgent.Logger.Error("Moving file to error folder - " + messagegenerator.ExtractErrorMessageFromErrorObject(err))
					if errorBlocks {
						go DATASTACKAgent.handleErrorFlowRequest(entry, properties, remoteTxnID, dataStackTxnID, messagegenerator.ExtractErrorMessageFromErrorObject(err))
					}
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
				DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.UPLOADREQUEST, metadatagenerator.GenerateFileUploadMetaData(originalFileName, originalAbsoluteFilePath, mirrorPath, uniqueRemoteTxn, uniqueRemoteTxnOptions.FileName, uniqueRemoteTxnOptions.Checksum, structureID, blockName, inputFolder, newFileName, newLocation, md5CheckSum, remoteTxnID, dataStackTxnID, properties), time.Now(), "IN", false)
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

func deAllocateInputDirectory(inputDir string) {
	delete(directoryAllocation, inputDir)
}

func (DATASTACKAgent *AgentDetails) handleErrorFlowRequest(entry models.TransferLedgerEntry, properties models.FlowWatcherProperties, remoteTxnID string, dataStackTxnID string, errMsg string) {
	DATASTACKAgent.Logger.Debug("Handling Error Flow Request %s", entry.FlowName)
	DATASTACKAgent.Logger.Debug("Entry details -: %v", entry)
	DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.ERRORFLOWAPIREQUEST, "", time.Now(), "OUT", false)

	errorFlowRequestData := models.ErrorFlowRequestData{
		AppName:    entry.AppName,
		AgentName:  DATASTACKAgent.AgentName,
		FlowID:     entry.FlowID,
		FlowName:   entry.FlowName,
		NodeType:   "INPUT",
		NodeID:     "1",
		Message:    errMsg,
		StackTrace: errMsg,
		StatusCode: "400",
	}

	payload, err := json.Marshal(errorFlowRequestData)
	if err != nil {
		DATASTACKAgent.Logger.Error("Error Flow Request Data Marshalling Error %s ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.ERRORFLOWAPIREQUESTERROR, "", time.Now(), "OUT", false)
		return
	}

	var client = DATASTACKAgent.fetchHTTPClient()
	req, _ := http.NewRequest("POST", DATASTACKAgent.BaseURL+"/errorFlowRequest", bytes.NewReader(payload))
	DATASTACKAgent.updateHeaders(&req.Header)
	req.Header.Set("AgentType", DATASTACKAgent.AgentType)
	req.Header.Set("AgentID", DATASTACKAgent.AgentID)
	req.Header.Set("DATA-STACK-App-Name", DATASTACKAgent.AppName)
	req.Header.Set("DATA-STACK-Flow-Name", entry.FlowName)
	req.Header.Set("DATA-STACK-Flow-Id", entry.FlowID)
	req.Header.Set("DATA-STACK-Remote-Txn-Id", remoteTxnID)
	req.Header.Set("DATA-STACK-Txn-Id", dataStackTxnID)
	req.Header.Set("DATA-STACK-Deployment-Name", entry.DeploymentName)
	req.Header.Set("namespace", entry.Namespace)
	req.Header.Set("DATA-STACK-Operating-System", runtime.GOOS)
	DATASTACKAgent.Logger.Info("Making request at %s", DATASTACKAgent.BaseURL+"/errorFlowRequest")
	DATASTACKAgent.Logger.Debug("Request Headers %v", req.Header)
	req.Close = true
	_, err = client.Do(req)
	if err != nil {
		DATASTACKAgent.Logger.Error("Error Flow API Request Error %s ", messagegenerator.ExtractErrorMessageFromErrorObject(err))
		DATASTACKAgent.addEntryToTransferLedger(entry.Namespace, entry.FlowName, entry.FlowID, entry.DeploymentName, ledgers.ERRORFLOWAPIREQUESTERROR, "", time.Now(), "OUT", false)
	}
}
