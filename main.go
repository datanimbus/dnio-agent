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
var DATASTACKAgent = agent.AgentDetails{}
var isInternalAgent string

type program struct {
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	data := Utils.ReadCentralConfFile(*confFilePath)
	if service.Interactive() {
		Logger.Info("Running in terminal.")
		p := verifyAgentPassword(*password)
		startAgent(*confFilePath, data, p, true)
	} else {
		Logger.Info("Running under service manager.")
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
	logClient := Utils.GetNewHTTPClient(nil)
	logsHookURL := "https://" + confData["base-url"] + "/logs"
	headers := map[string]string{}
	headers["DATA-STACK-Agent-Id"] = confData["agent-id"]
	headers["DATA-STACK-Agent-Name"] = confData["agent-name"]
	LoggerService := log.Logger{}
	Logger = LoggerService.GetLogger(confData["log-level"], confData["agent-name"], confData["agent-id"], "", "", "", logsHookURL, logClient, headers)

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

	confData := Utils.ReadCentralConfFile(*confFilePath)
	payload := models.LoginAPIRequest{
		AgentID:      confData["agent-id"],
		Password:     pass,
		AgentVersion: confData["agent-version"],
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = nil
		Logger.Error(err)
	}

	URL := "https://{BaseURL}/bm/auth/agent/login"
	URL = strings.Replace(URL, "{BaseURL}", confData["base-url"], -1)
	Logger.Info("Connecting to integration manager - " + URL)
	client := Utils.GetNewHTTPClient(nil)
	req, err := http.NewRequest("POST", URL, bytes.NewReader(data))
	if err != nil {
		data = nil
		Logger.Error("Error from Integration Manager - " + err.Error())
		os.Exit(0)
	}
	req.Close = true
	res, err := client.Do(req)
	if err != nil {
		data = nil
		Logger.Error("Error from Integration Manager - " + err.Error())
		os.Exit(0)
	}
	if res.StatusCode != 200 {
		data = nil
		if res.Body != nil {
			responseData, _ := ioutil.ReadAll(res.Body)
			Logger.Error("Error from Integration Manager - " + string(responseData))
			os.Exit(0)
		} else {
			Logger.Error("Error from Integration Manager - " + http.StatusText(res.StatusCode))
			os.Exit(0)
		}
	} else {
		bytesData, err := ioutil.ReadAll(res.Body)
		if err != nil {
			data = nil
			bytesData = nil
			Logger.Error("Error reading response body from IM - " + err.Error())
			os.Exit(0)
		}
		if res.Body != nil {
			res.Body.Close()
		}
		err = json.Unmarshal(bytesData, &BMResponse)
		if err != nil {
			data = nil
			bytesData = nil
			Logger.Error("Error unmarshalling response from IM - " + err.Error())
			os.Exit(0)
		}
		err = json.Unmarshal([]byte(BMResponse.AgentData), &DATASTACKAgent)
		if err != nil {
			data = nil
			bytesData = nil
			Logger.Error("Error unmarshalling agent data from IM - " + err.Error())
			os.Exit(0)
		}
		Logger.Info("Agent Successfuly Logged In - ", DATASTACKAgent.AgentName)
		Logger.Info("Agent Data - " + BMResponse.AgentData)
	}
	return string(pass)
}

func startAgent(confFilePath string, data map[string]string, password string, interactive bool) {
	//Setting up the agent
	DATASTACKAgent.AgentID = data["agent-id"]
	DATASTACKAgent.AgentName = data["agent-name"]
	DATASTACKAgent.AgentPortNumber = data["agent-port-number"]
	DATASTACKAgent.BaseURL = data["base-url"]
	DATASTACKAgent.HeartBeatFrequency = data["heartbeat-frequency"]
	DATASTACKAgent.LogLevel = data["log-level"]
	DATASTACKAgent.SentinelPortNumber = data["sentinel-port-number"]
	agent.SetUpAgent(data["central-folder"], &DATASTACKAgent, password, interactive)
	DATASTACKAgent.StartAgent()
}
