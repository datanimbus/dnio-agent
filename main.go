package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"ds-agent/agent"
	"ds-agent/utils"

	vault "github.com/appveen/govault"
	"github.com/howeyc/gopass"
	"github.com/kardianos/service"
)

var logger service.Logger
var vaultPassword *string
var encrypt = flag.Bool("encrypt", false, "Encryption tool")
var decrypt = flag.Bool("decrypt", false, "Decryption tool")
var svcFlag = flag.String("service", "", "Control the system service.")
var confFilePath = flag.String("c", "./conf/agent.conf", "Conf File Path")
var password = flag.String("p", "", "Vault Password")
var inputPath = flag.String("in", "", "input file path")
var outputPath = flag.String("out", "", "output file path")
var Utils = utils.UtilsService{}
var isInternalAgent string

type program struct {
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	data := Utils.ReadCentralConfFile(*confFilePath)
	if service.Interactive() {
		logger.Info("Running in terminal.")
		p := verifyAgentPassword(*password)
		startAgent(*confFilePath, data, p, true)
	} else {
		logger.Info("Running under service manager.")
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
	Utils := utils.UtilsService{}
	flag.Parse()

	d, _, _ := utils.GetExecutablePathAndName()
	confData := Utils.ReadCentralConfFile(*confFilePath)
	serviceName := "DATASTACKB2BAgent" + confData["agent-port-number"]
	if string(*password) != "" {
		verifyAgentPassword(*password)
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
		log.Fatal(err)
	}
	errs := make(chan error, 5)
	logger, err = s.Logger(errs)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			err := <-errs
			if err != nil {
				log.Print(err)
			}
		}
	}()

	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			if *svcFlag == "stop" && (err.Error() != "Failed to stop "+serviceName+": The specified service does not exist as an installed service." &&
				err.Error() != "Failed to stop "+serviceName+": The service has not been started.") {
				log.Printf("Action performed : %q\n", *svcFlag)
				log.Fatal(err)
			} else if *svcFlag == "uninstall" && err.Error() != "Failed to uninstall "+serviceName+": service "+serviceName+" is not installed" {
				log.Printf("Action performed : %q\n", *svcFlag)
				log.Fatal(err)
			}
		}
		if *svcFlag == "start" {
			log.Print(serviceName + " started succcessfully")
		} else if *svcFlag == "install" {
			log.Print(serviceName + " installed succcessfully")
		}
		return
	}
	err = s.Run()
	if err != nil {
		logger.Error(err)
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

	d, _, _ := utils.GetExecutablePathAndName()
	dataStackVault, err := vault.InitVault(filepath.Join(d, "..", "conf", "db.vault"), string(pass))
	if err != nil {
		fmt.Println("Incorrect vault password")
		os.Exit(1)
	}

	deleted, err := dataStackVault.Get("deleted")
	if string(deleted) == "true" {
		fmt.Println("This agent has been deleted. please download new agent")
		os.Exit(0)
	}
	dataStackVault.Close()

	return string(pass)
}

func startAgent(confFilePath string, data map[string]string, password string, interactive bool) {
	DATASTACKAgent := agent.AgentDetails{}
	DATASTACKAgent.AgentName = data["agent-name"]
	DATASTACKAgent.AgentID = data["agent-id"]
	DATASTACKAgent.AgentVersion = data["agent-version"]
	DATASTACKAgent.AppName = data["app-name"]
	DATASTACKAgent.UploadRetryCounter = data["upload-retry-counter"]
	DATASTACKAgent.DownloadRetryCounter = data["download-retry-counter"]
	DATASTACKAgent.BaseURL = data["base-url"]
	DATASTACKAgent.HeartBeatFrequency = data["heartbeat-frequency"]
	DATASTACKAgent.LogLevel = data["log-level"]
	DATASTACKAgent.AgentType = data["agent-type"]
	DATASTACKAgent.SentinelPortNumber = data["sentinel-port-number"]
	DATASTACKAgent.AgentPortNumber = data["agent-port-number"]
	DATASTACKAgent.SentinelMaxMissesCount = data["sentinel-max-misses-count"]
	DATASTACKAgent.MaxLogBackUpIndex = data["b2b_agent_log_retention_count"]
	DATASTACKAgent.LogMaxFileSize = data["b2b_agent_log_max_file_size"]
	DATASTACKAgent.LogRotationType = data["b2b_agent_log_rotation_type"]
	agent.SetUpAgent(data["central-folder"], &DATASTACKAgent, password, interactive)
	DATASTACKAgent.StartAgent()
}
