package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ds-agent/agent"
	"ds-agent/log"
	"ds-agent/models"
	"ds-agent/utils"

	"github.com/appveen/go-log/logger"
	"github.com/howeyc/gopass"
	"github.com/kardianos/service"
)

var svcLog service.Logger
var Logger logger.Logger
var encrypt = flag.Bool("encrypt", false, "Encryption tool")
var decrypt = flag.Bool("decrypt", false, "Decryption tool")
var svcFlag = flag.String("service", "", "Control the system service.")
var confFilePath = flag.String("c", "./conf/agent.conf", "Conf File Path")
var password = flag.String("p", "", "Password")
var inputPath = flag.String("in", "", "input file path")
var outputPath = flag.String("out", "", "output file path")
var Utils = utils.UtilsService{}
var BMResponse = models.LoginAPIResponse{}
var isInternalAgent string
var AgentDataFromIM = models.AgentData{}
var ipAddress string
var macAddress string

type program struct {
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	data := Utils.ReadCentralConfFile(*confFilePath)
	if service.Interactive() {
		svcLog.Info("Running in terminal.")
		p := verifyAgentPassword(*password)
		startAgent(*confFilePath, data, p, true)
	} else {
		svcLog.Info("Running under service manager.")
		p := verifyAgentPassword(*password)
		startAgent(*confFilePath, data, p, false)
	}
	p.exit = make(chan struct{})
	go p.run()
	return nil
}

func (p *program) run() error {
	return nil
}
func (p *program) Stop(s service.Service) error {
	close(p.exit)
	return nil
}

func main() {
	confData := Utils.ReadCentralConfFile(*confFilePath)
	isInternalAgent = "false"
	flag.Parse()
	if *encrypt {
		client := Utils.GetNewHTTPClient(nil)
		reqURL := "http://localhost:63859/encryptFile"
		if confData["agent-port-number"] != "" {
			reqURL = strings.Replace(reqURL, "63859", confData["agent-port-number"], -1)
		}
		if *password == "" || *inputPath == "" || *outputPath == "" {
			Logger.Error("Something is missing")
			os.Exit(0)
		}
		data := models.EncryptionDecryptionTool{}
		response := models.EncryptionDecryptionToolMessage{}
		data.Password = *password
		data.InputFilePath = *inputPath
		data.OutputFilePath = *outputPath
		err := Utils.MakeJSONRequest(client, reqURL, data, nil, &response)
		if err != nil {
			Logger.Error("%s", err)
			os.Exit(0)
		}
		Logger.Info(response.Message)
		os.Exit(0)
	} else if *decrypt {
		client := Utils.GetNewHTTPClient(nil)
		reqURL := "http://localhost:63859/decryptFile"
		if confData["agent-port-number"] != "" {
			reqURL = strings.Replace(reqURL, "63859", confData["agent-port-number"], -1)
		}
		if *password == "" || *inputPath == "" || *outputPath == "" {
			Logger.Error("something is missing")
			os.Exit(0)
		}
		data := models.EncryptionDecryptionTool{}
		response := models.EncryptionDecryptionToolMessage{}
		data.Password = *password
		data.InputFilePath = *inputPath
		data.OutputFilePath = *outputPath
		err := Utils.MakeJSONRequest(client, reqURL, data, nil, &response)
		if err != nil {
			Logger.Error("%s", err)
			os.Exit(0)
		}
		Logger.Info(response.Message)
		os.Exit(0)
	}

	d, _, _ := utils.GetExecutablePathAndName()
	serviceName := "DATASTACKB2BAgent"
	if isInternalAgent != "" && isInternalAgent == "true" {
		serviceName = serviceName + confData["agent-port-number"]
	}

	svcConfig := &service.Config{
		Name:        serviceName,
		DisplayName: serviceName,
		Description: "Agent For DATASTACK B2B",
		Arguments:   []string{"-p", *password, "-c", filepath.Join(d, "..", "conf", "agent.conf")},
	}
	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		Logger.Fatal(err)
	}

	errs := make(chan error, 5)
	svcLog, err = s.Logger(errs)
	if err != nil {
		Logger.Fatal(err)
	}
	go func() {
		for {
			err := <-errs
			if err != nil {
				Logger.Info(err)
			}
		}
	}()

	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			if *svcFlag == "stop" && (err.Error() != "Failed to stop "+serviceName+": The specified service does not exist as an installed service." &&
				err.Error() != "Failed to stop "+serviceName+": The service has not been started.") {
				Logger.Info("Action performed : %q\n", *svcFlag)
				Logger.Fatal(err)
			} else if *svcFlag == "uninstall" && err.Error() != "Failed to uninstall "+serviceName+": service "+serviceName+" is not installed" {
				Logger.Info("Action performed : %q\n", *svcFlag)
				Logger.Fatal(err)
			}
		}
		if *svcFlag == "start" {
			Logger.Info(serviceName + " started succcessfully")
		} else if *svcFlag == "install" {
			Logger.Info(serviceName + " installed succcessfully")
		}
		return
	}
	err = s.Run()
	if err != nil {
		Logger.Error(err)
	}
}

func verifyAgentPassword(password string) string {
	pass := ""
	if password == "" {
		fmt.Print("Enter Password : ")
		passBytes, _ := gopass.GetPasswdMasked()
		pass = string(passBytes)
	} else {
		pass = password
	}

	ipAddress = Utils.GetLocalIP()
	list, err := Utils.GetMacAddr()
	if err != nil {
		svcLog.Error(fmt.Sprintf("Mac Address fetching error -: %s", err))
		os.Exit(0)
	}
	macAddress = list[0]

	confData := Utils.ReadCentralConfFile(*confFilePath)
	payload := models.LoginAPIRequest{
		AgentID:      confData["agent-id"],
		Password:     pass,
		AgentVersion: confData["agent-version"],
		IPAddress:    ipAddress,
		MACAddress:   macAddress,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = nil
		svcLog.Error(err)
	}

	logClient := Utils.GetNewHTTPClient(nil)
	logsHookURL := "https://" + confData["base-url"] + "/b2b/bm/{App}/agent/utils/logs"
	headers := map[string]string{}
	LoggerService := log.Logger{}

	URL := "https://{BaseURL}/b2b/bm/auth/login"
	URL = strings.Replace(URL, "{BaseURL}", confData["base-url"], -1)
	svcLog.Info("Connecting to Integration Manager - " + URL)
	client := Utils.GetNewHTTPClient(nil)
	req, err := http.NewRequest("POST", URL, bytes.NewReader(data))
	if err != nil {
		data = nil
		svcLog.Error("Error from Integration Manager - " + err.Error())
		os.Exit(0)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Close = true
	res, err := client.Do(req)
	if err != nil {
		data = nil
		svcLog.Error("Error from Integration Manager - " + err.Error())
		os.Exit(0)
	}
	if res.StatusCode != 200 {
		data = nil
		if res.Body != nil {
			responseData, _ := ioutil.ReadAll(res.Body)
			err = json.Unmarshal(responseData, &BMResponse)
			if err != nil {
				responseData = nil
				svcLog.Error("Error unmarshalling response from IM - " + err.Error())
				os.Exit(0)
			}
			svcLog.Error("Error from Integration Manager - " + BMResponse.Message)
			os.Exit(0)
		} else {
			svcLog.Error("Error from Integration Manager - " + http.StatusText(res.StatusCode))
			os.Exit(0)
		}
	} else {
		bytesData, err := ioutil.ReadAll(res.Body)
		if err != nil {
			data = nil
			bytesData = nil
			svcLog.Error("Error reading response body from IM - " + err.Error())
			os.Exit(0)
		}
		if res.Body != nil {
			res.Body.Close()
		}
		err = json.Unmarshal([]byte(bytesData), &AgentDataFromIM)
		if err != nil {
			data = nil
			bytesData = nil
			svcLog.Error("Error unmarshalling agent data from IM - " + err.Error())
			os.Exit(0)
		}

		headers["DATA-STACK-Agent-Id"] = confData["agent-id"]
		headers["DATA-STACK-Agent-Name"] = confData["agent-name"]
		headers["DATA-STACK-App-Name"] = AgentDataFromIM.AppName
		headers["DATA-STACK-Mac-Address"] = macAddress
		headers["DATA-STACK-Ip-Address"] = ipAddress
		headers["Authorization"] = "JWT " + AgentDataFromIM.Token
		logsHookURL = strings.Replace(logsHookURL, "{App}", AgentDataFromIM.AppName, -1)
		Logger = LoggerService.GetLogger(confData["log-level"], confData["agent-name"], confData["agent-id"], "", "", "days", logsHookURL, logClient, headers)

		Logger.Info("Agent Successfuly Logged In")
		Logger.Trace("Agent details fetched -  %v ", AgentDataFromIM)
	}
	return string(pass)
}

func startAgent(confFilePath string, data map[string]string, password string, interactive bool) {
	//Setting up the agent
	DATASTACKAgent := agent.AgentDetails{}
	DATASTACKAgent.AgentID = data["agent-id"]
	DATASTACKAgent.AgentName = data["agent-name"]
	DATASTACKAgent.AgentPortNumber = data["agent-port-number"]
	DATASTACKAgent.BaseURL = data["base-url"]
	DATASTACKAgent.HeartBeatFrequency = data["heartbeat-frequency"]
	DATASTACKAgent.LogLevel = data["log-level"]
	DATASTACKAgent.SentinelPortNumber = data["sentinel-port-number"]
	DATASTACKAgent.EncryptFile = AgentDataFromIM.EncryptFile
	DATASTACKAgent.RetainFileOnSuccess = AgentDataFromIM.RetainFileOnSuccess
	DATASTACKAgent.RetainFileOnError = AgentDataFromIM.RetainFileOnError
	DATASTACKAgent.Token = AgentDataFromIM.Token
	DATASTACKAgent.AgentVersion = AgentDataFromIM.AgentVersion
	DATASTACKAgent.AppName = AgentDataFromIM.AppName
	DATASTACKAgent.EncryptionKey = AgentDataFromIM.EncryptionKey
	DATASTACKAgent.UploadRetryCounter = AgentDataFromIM.UploadRetryCounter
	DATASTACKAgent.DownloadRetryCounter = AgentDataFromIM.DownloadRetryCounter
	DATASTACKAgent.MaxConcurrentUploads = AgentDataFromIM.MaxConcurrentUploads
	DATASTACKAgent.MaxConcurrentDownloads = AgentDataFromIM.MaxConcurrentDownloads
	DATASTACKAgent.IPAddress = ipAddress
	DATASTACKAgent.MACAddress = macAddress
	agent.SetUpAgent(data["central-folder"], &DATASTACKAgent, password, interactive, Logger)
	DATASTACKAgent.StartAgent()
}
