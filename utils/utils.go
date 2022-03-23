package utils

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//UtilsService - struct
type UtilsService struct{}

//UtilityService - task of utils package
type UtilityService interface {
	GetLocalIP() string
	GetMacAddr() ([]string, error)
	GetNewHTTPClient(transport *http.Transport) *http.Client
	MakeJSONRequest(client *http.Client, url string, payload interface{}, headers map[string]string, response interface{}) error
	CheckFolderExistsOrCreateFolder(folder string) error
	StartHTTPServer(*http.Server) error
	ReadAll(r io.Reader) ([]byte, error)
}

//ReadCentralConfFile - read agent/edge conf file into map
func (Utils *UtilsService) ReadCentralConfFile(filePath string) map[string]string {
	data, err := Utils.ReadLinesToFile(filePath)
	if err != nil {
		panic(err)
	}
	mappedValues := make(map[string]string)
	for _, item := range data {
		values := strings.Split(item, "=")
		if len(values) == 2 {
			mappedValues[values[0]] = values[1]
		} else {
			mappedValues[values[0]] = ""
		}
	}
	return mappedValues
}

//ReadLinesToFile - Read lines from a file in string slice
func (Utils *UtilsService) ReadLinesToFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

//GetExecutablePathAndName - get executable path and name
func GetExecutablePathAndName() (string, string, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	d, f := filepath.Split(executablePath)
	return d, f, nil
}

//GetMacAddr - get list of mac addresses
func (Utils *UtilsService) GetMacAddr() ([]string, error) {
	ifas, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var as []string
	for _, ifa := range ifas {
		a := ifa.HardwareAddr.String()
		if a != "" {
			as = append(as, a)
		}
	}
	return as, nil
}

//GetLocalIP - get local ip address of machine
func (Utils *UtilsService) GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

//GetNewHTTPClient - get new http client with/without TLS
func (Utils *UtilsService) GetNewHTTPClient(transport *http.Transport) *http.Client {
	if transport != nil {
		return &http.Client{Transport: transport}
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &http.Client{Transport: tr}
}

//MakeJSONRequest - utility function to make JSON request
func (Utils *UtilsService) MakeJSONRequest(client *http.Client, url string, payload interface{}, headers map[string]string, response interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, er := http.NewRequest("POST", url, bytes.NewReader(data))
	if er != nil {
		data = nil
		return er
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Close = true
	res, errr := client.Do(req)
	if errr != nil {
		data = nil
		return errr
	}
	if res.StatusCode != 200 {
		if res.Body != nil {
			responseData, _ := ioutil.ReadAll(res.Body)
			return errors.New(string(responseData))
		} else {
			return errors.New("Request failed status code " + http.StatusText(res.StatusCode))
		}
	}
	bytesData, err := ioutil.ReadAll(res.Body)
	if res.Body != nil {
		res.Body.Close()
	}
	if err != nil {
		bytesData = nil
		data = nil
		return err
	}
	err = json.Unmarshal(bytesData, &response)
	if err != nil {
		bytesData = nil
		data = nil
		return err
	}
	bytesData = nil
	data = nil
	return nil
}

// CheckFolderExistsOrCreateFolder - checks if a folder exists at path or creates it
func (Utils *UtilsService) CheckFolderExistsOrCreateFolder(folder string) error {
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		errr := os.MkdirAll(folder, 0777)
		return errr
	}
	return nil
}

//StartHTTPServer - start the servers
func (Utils *UtilsService) StartHTTPServer(server *http.Server) error {
	err := server.ListenAndServe()
	return err
}

//ReadAll - for reading request or response body
func (Utils *UtilsService) ReadAll(r io.Reader) ([]byte, error) {
	return ioutil.ReadAll(r)
}
