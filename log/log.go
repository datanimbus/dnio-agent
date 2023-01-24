package log

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"path/filepath"

	"ds-agent/utils"

	"github.com/appveen/go-log/appenders"
	"github.com/appveen/go-log/layout"
	"github.com/appveen/go-log/levels"
	"github.com/appveen/go-log/log"
	"github.com/appveen/go-log/logger"
)

// LoggerService - provide log services
type LoggerService interface {
	GetLogger(agentType string, level string, agentName string, agentID string, MaxBackupIndex string, MaxFileSize string, LogRotationType string, logHookURL string, client *http.Client, headers map[string]string) logger.Logger
}

// Log - empty Log struct
type Logger struct{}

// GetLogger - get logger
func (Log *Logger) GetLogger(level string, agentName string, agentID string, MaxBackupIndex string, MaxFileSize string, LogRotationType string, logHookURL string, client *http.Client, headers map[string]string) logger.Logger {
	logger := log.Logger("logger")
	pattern := "{\"timestamp\":" + "\"%d\"," + "\"level\":" + "\"%p\","
	mode := strings.ToUpper(level)
	loggingLevel := levels.INFO
	switch mode {
	case "INFO":
		loggingLevel = levels.INFO
	case "DEBUG":
		loggingLevel = levels.DEBUG
	case "TRACE":
		loggingLevel = levels.TRACE
	case "PROD":
		loggingLevel = levels.INFO
	default:
		loggingLevel = levels.INFO
	}
	logger.SetLevel(loggingLevel)
	if IsKubernetesEnv() {
		pattern += "\"hostname\":" + os.Getenv("HOSTNAME") + "\","
	}
	pattern += "\"msg\":" + "\"%m\"}"
	layoutPattern := layout.Pattern(pattern)
	consoleAppender := appenders.Console()
	consoleAppender.SetLayout(layoutPattern)
	maxBackupIndex, err := strconv.Atoi(MaxBackupIndex)
	if err != nil || maxBackupIndex == 0 {
		maxBackupIndex = 10
	}
	maxFileSize, err := strconv.Atoi(MaxFileSize)
	if err != nil || maxFileSize == 0 {
		maxFileSize = 104857600
	}
	maxBackupIndex++
	if !(IsKubernetesEnv()) {
		fileName := "agent.log"
		d, _, _ := utils.GetExecutablePathAndName()
		rollingFileAppender := appenders.RollingFile(filepath.Join(d, "..", "log", fileName), filepath.Join(d, "..", "log"), fileName, true, true, true, 1, "", nil)
		if strings.ToUpper(LogRotationType) == "SIZE" {
			rollingFileAppender = nil
			rollingFileAppender = appenders.RollingFile(filepath.Join(d, "..", "log", fileName), filepath.Join(d, "..", "log"), fileName, true, false, true, maxBackupIndex, "", nil)
			rollingFileAppender.MaxFileSize = int64(maxFileSize)
		}
		rollingFileAppender.SetLayout(layoutPattern)
		currentTime := time.Now()
		dateFileName := "agent_" + currentTime.Format("2006-01-02") + ".log"
		dateFileAppender := appenders.RollingFile(filepath.Join(d, "..", "log", dateFileName), filepath.Join(d, "..", "log"), dateFileName, true, true, false, maxBackupIndex, "", nil)
		dateFileAppender.MaxBackupIndex = maxBackupIndex
		dateFileAppender.SetLayout(layoutPattern)
		logShippingFileSize := 1024
		namer := func() string {
			return "temp.log"
		}
		logShippingFileAppender := appenders.RollingFile(filepath.Join(d, "..", "log", "temp.log"), filepath.Join(d, "..", "log"), "temp.log", true, false, true, 102400, filepath.Join(d, "..", "log", "temp"), namer)
		logShippingFileAppender.MaxFileSize = int64(logShippingFileSize)
		logShippingFileAppender.MaxBackupIndex = 102400
		logShippingFileAppender.LogHookURL = logHookURL
		logShippingFileAppender.Client = client
		logShippingFileAppender.CustomHeaders = headers
		logShippingFileAppender.SetLayout(layoutPattern)
		if strings.ToUpper(LogRotationType) == "DAYS" {
			logger.SetAppender(appenders.Multiple(layoutPattern, consoleAppender, rollingFileAppender, dateFileAppender, logShippingFileAppender))
		} else {
			dateFileAppender = nil
			logger.SetAppender(appenders.Multiple(layoutPattern, consoleAppender, rollingFileAppender, logShippingFileAppender))
			os.Remove(filepath.Join(d, "..", "log", dateFileName))
		}
		return logger
	}
	logger.SetAppender(consoleAppender)
	return logger
}

// IsKubernetesEnv - is kubernetes environment
func IsKubernetesEnv() bool {
	if os.Getenv("KUBERNETES_SERVICE_PORT") != "" && os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}
	return false
}
