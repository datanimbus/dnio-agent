package utils

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"ds-agent/messagegenerator"
	"ds-agent/models"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
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
	CreateOrUpdateFlowConfFile(filePath string, flow *models.FlowDefinitionResponse, running bool, DeploymentName string) error
	ReadFlowConfigurationFile(filePath string) (map[string]string, error)
	WriteLinesToFile(lines []string, path string) error
	CheckFileRegexAndExtension(fileName string, fileExtensions []models.FileExtensionStruct, fileNameRegexs []string) bool
	CreateFlowErrorFile(filePath string, errorMsg string) error
	GetFileDetails(filePath string, AppFolder string, FlowName string, fileExtensions []models.FileExtensionStruct, fileNameRegexs []string) (flowName string, originalFileName string, originalAbsoluteFilePath string, newFileName string, newLocation string, md5CheckSum string, err error)
	CalculateMD5ChecksumForFile(filePath string) (string, error)
	Compress(data []byte) []byte
	Decompress(data []byte) []byte
	EncryptData(data []byte, passphrase string) ([]byte, error)
	DecryptData(data []byte, passphrase string) ([]byte, error)
	Get64BitBinaryStringNumber(number int64) string
	CalculateMD5ChecksumForByteSlice(data []byte) string
	DecryptFileInChunksAndWriteInOutputFile(inputPath string, outputPath string, password string, bytesToSkip int) error
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
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
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

//CreateOrUpdateFlowConfFile - function to create flow configuration file
func (Utils *UtilsService) CreateOrUpdateFlowConfFile(filePath string, flow *models.FlowDefinitionResponse, running bool, DeploymentName string) error {
	data := []string{}
	data = append(data, "flow-name="+flow.FlowName)
	data = append(data, "flow-id="+flow.FlowID)
	data = append(data, "file-suffix="+flow.FileSuffix)
	data = append(data, "deployment-name="+DeploymentName)
	data = append(data, "max-file-size="+strconv.Itoa(flow.FileMaxSize))
	if running {
		data = append(data, "running=YES")
	} else {
		data = append(data, "running=NO")
	}
	err := Utils.WriteLinesToFile(data, filePath)
	if err != nil {
		return err
	}
	return nil
}

//WriteLinesToFile - write lines to a file from string slice
func (Utils *UtilsService) WriteLinesToFile(lines []string, path string) error {
	file, err := os.Create(path)
	file.Truncate(0)
	file.Seek(0, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	w := bufio.NewWriter(file)
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
	return w.Flush()
}

// ReadFlowConfigurationFile - read the "flow.conf" file present in flow folder
func (Utils *UtilsService) ReadFlowConfigurationFile(filePath string) (map[string]string, error) {
	values := map[string]string{}
	file, err := os.Open(filePath)
	if err != nil {
		return values, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		items := strings.Split(line, "=")
		if len(items) == 2 {
			values[items[0]] = items[1]
		}
	}
	return values, nil
}

//CheckFileRegexAndExtension - matching file regex and extension for file upload
func (Utils *UtilsService) CheckFileRegexAndExtension(fileName string, fileExtensions []models.FileExtensionStruct, fileNameRegexs []string) bool {
	matched := false
	matched1 := false
	matched2 := false
	items := strings.Split(fileName, ".")
	inputFileExtension := items[len(items)-1]

	if len(fileNameRegexs) == 0 {
		matched1 = true
	} else {
		for _, regex := range fileNameRegexs {
			regex = regex[1 : len(regex)-1]
			matched1, _ = regexp.MatchString(regex, items[0])
			if matched1 {
				break
			}
		}
	}

	if len(fileExtensions) == 0 {
		matched2 = true
	} else {
		for _, fileExtension := range fileExtensions {
			if fileExtension.Extension[0] == '.' {
				fileExtension.Extension = fileExtension.Extension[1:len(fileExtension.Extension)]
			}
			matched2, _ = regexp.MatchString("(?i)"+fileExtension.Extension, inputFileExtension)
			if matched2 {
				break
			}
		}
	}
	if matched1 && matched2 {
		matched = true
	}
	return matched
}

//CreateFlowErrorFile - function to create error file
func (Utils *UtilsService) CreateFlowErrorFile(filePath string, errorMsg string) error {
	data := []string{}
	if !strings.Contains(errorMsg, "Error-:") {
		data = append(data, "Error-:"+errorMsg)
	}
	err := Utils.WriteLinesToFile(data, filePath)
	if err != nil {
		return err
	}
	return nil
}

//GetFileDetails function for fetching details of file
func (Utils *UtilsService) GetFileDetails(filePath string, AppFolder string, FlowName string, fileExtensions []models.FileExtensionStruct, fileNameRegexs []string) (flowName string, originalFileName string, originalAbsoluteFilePath string, newFileName string, newLocation string, md5CheckSum string, err error) {
	items := strings.Split(filePath, string(os.PathSeparator))
	originalFileName = items[len(items)-1]
	flowFolder := AppFolder + string(os.PathSeparator) + strings.Replace(FlowName, " ", "_", -1)
	values, err := Utils.ReadFlowConfigurationFile(flowFolder + string(os.PathSeparator) + "flow.conf")
	if err != nil {
		return "", originalFileName, "", "", "", "", err
	}
	flowName = values["flow-name"]
	fileExtension := strings.Split(items[len(items)-1], ".")
	matched, err := regexp.MatchString("(?i)"+values["file-suffix"], fileExtension[len(fileExtension)-1])
	if err != nil {
		return "", originalFileName, "", "", "", "", err
	}

	if !matched {
		return "", originalFileName, "", "", "", "", errors.New("Incorrect File Added For Flow - " + flowName)
	}

	matched = Utils.CheckFileRegexAndExtension(originalFileName, fileExtensions, fileNameRegexs)
	if !matched {
		return "", originalFileName, "", "", "", "", errors.New("Incorrect File Added For Flow - " + flowName)
	}

	if runtime.GOOS == "windows" && len(originalFileName) > 100 {
		originalAbsoluteFilePath = filePath
		newFileName = originalFileName
		newLocation = flowFolder + string(os.PathSeparator) + "error"
		md5CheckSum = ""
		return flowName, originalFileName, originalAbsoluteFilePath, newFileName, newLocation, md5CheckSum, errors.New(messagegenerator.GetErrorMessageForLargeFileName(originalFileName))
	}
	maxFileSize, errr := strconv.Atoi(values["max-file-size"])
	if errr != nil {
		maxFileSize = int(104857600)
	}
	if maxFileSize == 0 {
		maxFileSize = int(104857600)
	}
	filesize, matched, validateSizeError := ValidateFileSize(filePath, maxFileSize)
	if validateSizeError != nil {
		originalAbsoluteFilePath = filePath
		newFileName = originalFileName
		newLocation = flowFolder + string(os.PathSeparator) + "error"
		md5CheckSum = ""
		return flowName, originalFileName, originalAbsoluteFilePath, newFileName, newLocation, md5CheckSum, validateSizeError
	}

	if !matched {
		originalAbsoluteFilePath = filePath
		newFileName = originalFileName
		newLocation = flowFolder + string(os.PathSeparator) + "error"
		md5CheckSum = ""
		return flowName, originalFileName, originalAbsoluteFilePath, newFileName, newLocation, md5CheckSum, errors.New(messagegenerator.GetErrorMessageForLargeFileSize(originalFileName, filesize, int64(maxFileSize)))
	}

	originalAbsoluteFilePath = filePath
	newFileName = time.Now().Format("2006_01_02_15_04_05_000000") + "____" + originalFileName + "processing"
	newLocation = flowFolder + string(os.PathSeparator) + "processing" + string(os.PathSeparator) + newFileName
	md5CheckSum = ""
	return flowName, originalFileName, originalAbsoluteFilePath, newFileName, newLocation, md5CheckSum, nil
}

//ValidateFileSize - validate file size
func ValidateFileSize(filepath string, maxFileSize int) (int64, bool, error) {
	previousTime := time.Now()
	for {
		time.Sleep(time.Second * 1)
		fileInfo, errr := os.Stat(filepath)
		if errr != nil {
			return 0, false, errr
		}
		currentTime := fileInfo.ModTime()
		if previousTime == currentTime {
			break
		}
		previousTime = currentTime
	}

	fileinfo, err := os.Stat(filepath)
	if err != nil {
		return 0, false, err
	}

	fileSize := fileinfo.Size()

	if fileSize > int64(maxFileSize) {
		return fileSize, false, nil
	} else {
		return fileSize, true, nil
	}

}

// CalculateMD5ChecksumForFile - calculate MD5 checksum for file content
func (Utils *UtilsService) CalculateMD5ChecksumForFile(filePath string) (string, error) {
	var returnMD5String string
	file, err := os.Open(filePath)
	if err != nil {
		return returnMD5String, err
	}
	defer file.Close()
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	hashInBytes := hash.Sum(nil)[:16]
	returnMD5String = hex.EncodeToString(hashInBytes)
	return returnMD5String, nil
}

//Compress - compressing file chunk
func (Utils *UtilsService) Compress(data []byte) []byte {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, _ = w.Write(data)
	w.Close()
	compressedData, _ := ioutil.ReadAll(&b)
	return []byte(base64.StdEncoding.EncodeToString(compressedData))
}

//Decompress - decompressing file chunk
func (Utils *UtilsService) Decompress(data []byte) []byte {
	b := bytes.NewBuffer(data)
	r, _ := zlib.NewReader(b)
	decompressedData, _ := ioutil.ReadAll(r)
	r.Close()
	return decompressedData
}

func createHash(key string) string {
	hasher := md5.New()
	hasher.Write([]byte(key))
	return hex.EncodeToString(hasher.Sum(nil)[:16])
}

//EncryptData - used for encryption of data
func (Utils *UtilsService) EncryptData(data []byte, passphrase string) ([]byte, error) {
	block, _ := aes.NewCipher([]byte(createHash(passphrase)))
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return []byte(base64.StdEncoding.EncodeToString(ciphertext)), nil
}

//DecryptData - used for decryption of data
func (Utils *UtilsService) DecryptData(data []byte, passphrase string) ([]byte, error) {
	key := []byte(createHash(passphrase))
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	decodedData, err := base64.StdEncoding.DecodeString(string(plaintext))
	if err != nil {
		fmt.Println("Decoding Data Error = ", err)
		return nil, err
	}
	return decodedData, nil
}

//Get64BitBinaryStringNumber - convert a int64 number to 64 bit string
func (Utils *UtilsService) Get64BitBinaryStringNumber(number int64) string {

	binarySize := strconv.FormatInt(number, 2)
	paddedZero := ""

	for i := 1; i <= 64-len(binarySize); i++ {
		paddedZero += "0"
	}

	binarySize = paddedZero + binarySize
	return binarySize
}

//CalculateMD5ChecksumForByteSlice - calculate MD5 checksum for byte slice
func (Utils *UtilsService) CalculateMD5ChecksumForByteSlice(data []byte) string {
	MD5 := md5.Sum(data)
	MD5String := fmt.Sprintf("%x", MD5)
	return MD5String
}

//DecryptFileInChunksAndWriteInOutputFile - buffered decryption and write it in output file, mainly command line request to agent
func (Utils *UtilsService) DecryptFileInChunksAndWriteInOutputFile(inputPath string, outputPath string, password string, bytesToSkip int) error {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Error occured in encrypting the file error is -: ", r)
		}
	}()

	inputFile, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	lengthReadBuffer := make([]byte, bytesToSkip)
	_, err = inputFile.Read(lengthReadBuffer)

	if err != nil {
		inputFile.Close()
		return err
	}
	inputFile.Close()

	inputFile, err = os.Open(inputPath)
	if err != nil {
		return err
	}

	if !IsBufferedEncryption(lengthReadBuffer) {
		previousEncryptedData, err := ioutil.ReadAll(inputFile)
		if err != nil {
			return err
		}
		decryptedData, err := Utils.DecryptData(previousEncryptedData, password)
		if err != nil {
			return err
		}
		_, err = outputFile.Write(decryptedData)
		if err != nil {
			return err
		}
		err = outputFile.Close()
		if err != nil {
			return err
		}
		err = inputFile.Close()
		if err != nil {
			return err
		}
		return nil
	}

	for {
		_, err := inputFile.Read(lengthReadBuffer)
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
		encryptedChunkSize, err := strconv.ParseInt(string(lengthReadBuffer), 2, 64)
		if err != nil {
			return err
		}
		encryptedDataBuffer := make([]byte, int64(encryptedChunkSize))
		_, err = inputFile.Read(encryptedDataBuffer)
		if err != nil {
			return err
		}

		compressedChunk, err := Utils.DecryptData(encryptedDataBuffer, password)
		decryptedData := Utils.Decompress(compressedChunk)
		if err != nil {
			return err
		}
		_, err = outputFile.Write(decryptedData)
		if err != nil {
			return err
		}
		encryptedDataBuffer = nil
	}
	err = outputFile.Close()
	if err != nil {
		return err
	}
	err = inputFile.Close()
	if err != nil {
		return err
	}
	return nil
}

func IsBufferedEncryption(data []byte) bool {
	if len(data) < 64 {
		return false
	}
	_, err := strconv.ParseInt(string(data), 2, 64)
	if err != nil {
		return false
	}
	return true
}
