package agent

import (
	"ds-agent/log"
	"ds-agent/utils"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/appveen/go-log/logger"
	vault "github.com/appveen/govault"
)

//AgentService - task of agent
type AgentService interface {
	StartAgent() error
	FetchAgentDetailsFromVault() error
	FetchHTTPClient() *http.Client
}

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
	DATASTACKAgent.FetchAgentDetailsFromVault()

	if DATASTACKAgent.BaseURL != "" && !strings.Contains(DATASTACKAgent.BaseURL, "https") {
		DATASTACKAgent.BaseURL = "https://" + DATASTACKAgent.BaseURL
	}

	logClient := DATASTACKAgent.FetchHTTPClient()
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
	return nil
}

//StartAgent - Entry point function to invoke remote machine agent for file upload/download
func (DATASTACKAgent *AgentDetails) StartAgent() error {
	var wg sync.WaitGroup
	DATASTACKAgent.Logger.Info("Agent Starting ...")
	DATASTACKAgent.Logger.Debug("AgentID- %s", DATASTACKAgent.AgentID)
	DATASTACKAgent.Logger.Info("AgentName- %s", DATASTACKAgent.AgentName)
	DATASTACKAgent.Logger.Info("Agent Type- %s", DATASTACKAgent.AgentType)
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
	wg.Wait()
	return nil
}

//FetchAgentDetailsFromVault - fetch agent details from vault
func (DATASTACKAgent *AgentDetails) FetchAgentDetailsFromVault() error {
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

//FetchHTTPClient -  getting http client with all certs
func (DATASTACKAgent *AgentDetails) FetchHTTPClient() *http.Client {
	var client *http.Client
	if len(DATASTACKAgent.DATASTACKCertFile) != 0 && len(DATASTACKAgent.DATASTACKKeyFile) != 0 {
		client = DATASTACKAgent.Utils.GetNewHTTPClient(DATASTACKAgent.Utils.PrepareTLSTransportConfigWithEncodedTrustStore(DATASTACKAgent.DATASTACKCertFile, DATASTACKAgent.DATASTACKKeyFile, DATASTACKAgent.TrustCerts))
	} else {
		client = DATASTACKAgent.Utils.GetNewHTTPClient(nil)
	}
	return client
}
