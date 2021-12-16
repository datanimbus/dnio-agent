package utils

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"log"
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
	PrepareTLSTransportConfigWithEncodedTrustStore(certFile []byte, keyFile []byte, trustCerts []string) *http.Transport
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

//PrepareTLSTransportConfigWithEncodedTrustStore - preparing tls config using encoded trust store
func (Utils *UtilsService) PrepareTLSTransportConfigWithEncodedTrustStore(certFile []byte, keyFile []byte, trustCerts []string) *http.Transport {
	cert, err := tls.X509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatal("Error1 : ", err)
	}
	caCertPool := x509.NewCertPool()
	for i := 0; i < len(trustCerts); i++ {
		currentEncodedCert := trustCerts[i]
		decodedCert, err := base64.StdEncoding.DecodeString(currentEncodedCert)
		if err != nil {
			log.Fatal("Error1 : ", err)
		}
		caCertPool.AppendCertsFromPEM(decodedCert)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}
	tlsConfig.InsecureSkipVerify = true
	tlsConfig.BuildNameToCertificate()
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	return transport
}
