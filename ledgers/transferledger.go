package ledgers

import (
	"ds-agent/models"
	"fmt"
	"os"
	"strings"

	"github.com/asdine/storm"
	"github.com/asdine/storm/q"
	uuid "github.com/satori/go.uuid"
)

const (
	//AGENTSTARTERROR - error occured on agent start
	AGENTSTARTERROR = "AGENT_START_ERROR"

	//HEARTBEATERROR - error occurred in either process
	HEARTBEATERROR = "HEARTBEAT_ERROR"

	//FLOWCREATEREQUEST - Action to create new flow request
	FLOWCREATEREQUEST = "FLOW_CREATE_REQUEST"

	//FLOWSTARTREQUEST - Action to start existing stopped flow
	FLOWSTARTREQUEST = "FLOW_START_REQUEST"

	//FLOWALREADYSTARTED - flow initiation after ieg or agent restart
	FLOWALREADYSTARTED = "FLOW_ALREADY_STARTED"

	//FLOWUPDATEREQUEST - status for flow update request
	FLOWUPDATEREQUEST = "FLOW_UPDATE_REQUEST"

	//FLOWCREATIONERROR - error occurred in flow creation
	FLOWCREATIONERROR = "FLOW_CREATION_ERROR"

	//FLOWCONFWRITEERROR - flow conf write error
	FLOWCONFWRITEERROR = "FLOW_CONF_WRITE_ERROR"

	//FLOWCONFREADERROR - flow conf write error
	FLOWCONFREADERROR = "FLOW_CONF_READ_ERROR"

	//FLOWCREATED - status when flow is created successfully
	FLOWCREATED = "FLOW_CREATED"

	//FLOWSTARTED - Flow watcher successfully started to upload files
	FLOWSTARTED = "FLOW_STARTED"

	//FLOWUPDATED - status when flow updated successfully
	FLOWUPDATED = "FLOW_UPDATED"

	//FLOWSTOPERROR - error occurred in flow stop action
	FLOWSTOPERROR = "FLOW_STOP_ERROR"

	//FLOWSTOPPED - Flow watcher stopped
	FLOWSTOPPED = "FLOW_STOPPED"

	//WATCHERERROR - error occurred in running watcher for flow
	WATCHERERROR = "WATCHER_ERROR"

	//GETMIRRORFOLDERSTRUCTURE = "get mirror structure from the input"
	GETMIRRORFOLDERSTRUCTURE = "GET_MIRROR_FOLDER_STRUCTURE"

	//GETMIRRORFOLDERSTRUCTUREERROR = "get mirror structure from the input error"
	GETMIRRORFOLDERSTRUCTUREERROR = "GET_MIRROR_FOLDER_STRUCTURE_ERROR"

	//RECEIVEMIRRORFOLDERSTRUCTURE = "receive mirror folder structure for mirroring at output"
	RECEIVEMIRRORFOLDERSTRUCTURE = "RECEIVE_MIRROR_FOLDER_STRUCTURE"

	//RECEIVEMIRRORFOLDERSTRUCTUREERROR = "receive mirror folder structure for mirroring at output error"
	RECEIVEMIRRORFOLDERSTRUCTUREERROR = "RECEIVE_MIRROR_FOLDER_STRUCTURE_ERROR"

	//FILEUPLOADERRORDUETOLARGEFILENAMEORMAXFILESIZE - file upload error due to large file name or max file size
	FILEUPLOADERRORDUETOLARGEFILENAMEORMAXFILESIZE = "FILE_UPLOAD_ERROR_DUE_TO_LARGE_FILE_NAME_MAX_FILE_SIZE"

	//FILEUPLOADERRORDUETOLARGEFILESIZE - file upload error due to large file size
	FILEUPLOADERRORDUETOLARGEFILESIZE = "FILE_UPLOAD_ERROR_DUE_TO_LARGE_FILE_SIZE"

	//ERRORFLOWAPIREQUEST - "agent side file error to error flow"
	ERRORFLOWAPIREQUEST = "ERROR_FLOW_API_REQUEST"

	//ERRORFLOWAPIREQUESTERROR - "error during error flow api request"
	ERRORFLOWAPIREQUESTERROR = "ERROR_FLOW_API_REQUEST_ERROR"

	//ERRORFLOWAPIREQUESTSUCCESSFULL - "agent side file error to error flow success"
	ERRORFLOWAPIREQUESTSUCCESSFULL = "ERROR_FLOW_API_REQUEST_SUCCESSFULL"

	//PREPROCESSINGFILEERROR - pre processing file error
	PREPROCESSINGFILEERROR = "PRE_PROCESSING_FILE_ERROR"

	//UPLOADREQUEST - status while file is added to .input folder
	UPLOADREQUEST = "UPLOADREQUEST"

	//REDOWNLOADFILEREQUEST - request for redownloading the file
	REDOWNLOADFILEREQUEST = "REDOWNLOAD_FILE_REQUEST"

	//QUEUEDJOBSERROR - error occurred in handling of queued jobs handling
	QUEUEDJOBSERROR = "QUEUED_JOBS_ERROR"

	//FLOWSTOPREQUEST - Action to stop existing flow
	FLOWSTOPREQUEST = "FLOW_STOP_REQUEST"

	//DOWNLOADREQUEST - status when file download request is received from edge/datastack-endpoint
	DOWNLOADREQUEST = "DOWNLOAD_REQUEST"

	//UPLOADERROR - error occurred in either process
	UPLOADERROR = "UPLOAD_ERROR"

	//UPLOADING - status while file is getting uploaded
	UPLOADING = "UPLOADING"

	//UPLOADED - status for successful file upload
	UPLOADED = "UPLOADED"

	//FILEUPLOADTOBMERROR - file upload to bm error
	FILEUPLOADTOBMERROR = "FILE_UPLOAD_TO_BM_ERROR"

	//DOWNLOADERROR - error occurred in either process
	DOWNLOADERROR = "DOWNLOAD_ERROR"

	//OUTPUTDIRECTORYDOESNOTEXISTERROR - output directory doesn't exist error
	OUTPUTDIRECTORYDOESNOTEXISTERROR = "OUTPUT_DIRECTORY_DOES_NOT_EXIST_ERROR"

	//DOWNLOADED - status when file has been successfully downloaded
	DOWNLOADED = "DOWNLOADED"

	//REDOWNLOADED - status when file get redownloaded
	REDOWNLOADED = "REDOWNLOADED"

	//CREATEAPIFLOWREQUEST - status when new api flow needs to be created
	CREATEAPIFLOWREQUEST = "CREATE_API_FLOW_REQUEST"

	//STOPAPIFLOWREQUEST - status when new api flow needs to be stopped
	STOPAPIFLOWREQUEST = "STOP_API_FLOW_REQUEST"

	//FILEPROCESSEDSUCCESS - Action when file processed successfully from DATASTACK
	FILEPROCESSEDSUCCESS = "FILE_PROCESSED_SUCCESS"

	//FILEPROCESSEDERROR - Action when file processed unsuccessfully from DATASTACK
	FILEPROCESSEDERROR = "FILE_PROCESSED_ERROR"

	//POSTPROCESSSUCCESSERROR - error occurred in handling of file after processed message received
	POSTPROCESSSUCCESSERROR = "POST_PROCESS_SUCCESS_ERROR"

	//POSTPROCESSFAILUREERROR - error occurred in handling of file after processing failure message received
	POSTPROCESSFAILUREERROR = "POST_PROCESS_FAILURE_ERROR"

	//DELETEFLOWREQUEST - action to delete existing flow
	DELETEFLOWREQUEST = "DELETE_FLOW_REQUEST"

	//STOPAGENT - stop agent
	STOPAGENT = "AGENT-STOPPED"

	//DISABLEAGENT - disable agent
	DISABLEAGENT = "AGENT-DISABLED"

	//PASSWORDCHANGED - password changed
	PASSWORDCHANGED = "PASSWORD-CHANGED"

	//DELETEAGENT - delete agent exe and related conf files
	DELETEAGENT = "DELETE_AGENT"
)

// TransferLedger - Base DB struct
type TransferLedger struct {
	DB                     *storm.DB
	STORE                  string
	TransferLedgerFilePath string
	TransferLedgerDBSize   int64
}

type TransferLedgerService interface {
	AddEntry(*models.TransferLedgerEntry) error
	GetUnsentNotifications() ([]models.TransferLedgerEntry, error)
	UpdateSentOrReadFieldOfEntry(entry *models.TransferLedgerEntry, val bool) error
	DeletePendingUploadRequestFromDB() (bool, error)
	CompactDB() error
	GetQueuedOperations(readCountLimit int) ([]models.TransferLedgerEntry, error)
	GetFileUploadRequests(readCountLimit int) ([]models.TransferLedgerEntry, error)
	GetFileDownloadRequests(readCountLimit int) ([]models.TransferLedgerEntry, error)
	IsFileAlreadyDownloaded(fileID string) bool
}

// InitTransferLedger - initialize DB
func InitTransferLedger(filePath string, store string) (*TransferLedger, error) {
	db, err := storm.Open(filePath)
	if err != nil {
		return nil, err
	}
	transferLedger := TransferLedger{
		DB: db,
	}
	transferLedger.TransferLedgerFilePath = filePath
	transferLedger.TransferLedgerDBSize = int64(32768)
	transferLedger.DB.Init(&models.TransferLedgerEntry{})
	return &transferLedger, nil
}

// AddEntry - add new entry to transfer ledger
func (db *TransferLedger) AddEntry(entry *models.TransferLedgerEntry) error {
	if entry.ID == "" {
		u := uuid.NewV4()
		entry.ID = u.String()
	}
	err := db.DB.Save(entry)
	if err != nil {
		return err
	}
	transferLedgerDBFileStat, err := os.Stat(db.TransferLedgerFilePath)
	if err == nil {
		n := transferLedgerDBFileStat.Size()
		if n >= db.TransferLedgerDBSize {
			err = db.handleLogsPurgeRequest()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (db *TransferLedger) handleLogsPurgeRequest() error {
	query := db.DB.Select(q.Eq("SentOrRead", true))
	err := query.Delete(new(models.TransferLedgerEntry))
	if err != nil && !strings.Contains(fmt.Sprintf("%v", err), "not found") {
		return err
	}
	return nil
}

// GetUnsentNotifications - get list of notifications to be sent
func (db *TransferLedger) GetUnsentNotifications() ([]models.TransferLedgerEntry, error) {
	transferLedgerEntries := []models.TransferLedgerEntry{}
	query := db.DB.Select(q.Eq("EntryType", "OUT"), q.Eq("SentOrRead", false)).Limit(500)
	err := query.Find(&transferLedgerEntries)
	if fmt.Sprintf("%s", err) == "not found" {
		return transferLedgerEntries, nil
	}
	if err != nil {
		return nil, err
	}
	return transferLedgerEntries, nil
}

// UpdateSentOrReadFieldOfEntry - update an existing entry field to transfer ledger
func (db *TransferLedger) UpdateSentOrReadFieldOfEntry(entry *models.TransferLedgerEntry, val bool) error {
	err := db.DB.UpdateField(entry, "SentOrRead", val)
	if err != nil {
		// fmt.Println("Update sent or read field error ", err)
		if strings.Contains(fmt.Sprintf("%s", err), "not found") {
			entry.SentOrRead = false
			err = db.DB.Save(entry)
			if err != nil {
				return err
			}
		}
		return err
	}
	return nil
}

// DeletePendingUploadRequestFromDB - deleting pending upload request from db on agent start
func (db *TransferLedger) DeletePendingUploadRequestFromDB() (bool, error) {
	query := db.DB.Select(q.Eq("SentOrRead", false), q.Eq("Action", UPLOADREQUEST))
	err := query.Delete(new(models.TransferLedgerEntry))

	if err != nil {
		if fmt.Sprintf("%s", err) == "not found" {
			return true, nil
		} else {
			return false, err
		}
	}

	return true, nil
}

// CompactDB - reduce DB size
func (db *TransferLedger) CompactDB() error {
	var entries []models.TransferLedgerEntry
	err := db.DB.Find("SentOrRead", false, &entries)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}
	db.DB.Close()
	newFilePath := strings.Replace(db.TransferLedgerFilePath, "transfer-ledger.db", "new-transfer-ledger.db", -1)
	dummyDB, err := storm.Open(newFilePath)
	dummyDB.Init(&models.TransferLedgerEntry{})
	if err != nil {
		return err
	}
	for _, entry := range entries {
		dummyDB.Save(&entry)
	}
	dummyDB.Close()
	err = os.Rename(newFilePath, db.TransferLedgerFilePath)
	if err != nil {
		return err
	}
	db.DB, _ = storm.Open(db.TransferLedgerFilePath)
	return nil
}

// GetQueuedOperations - update an existing entry to transfer ledger
func (db *TransferLedger) GetQueuedOperations(readCountLimit int) ([]models.TransferLedgerEntry, error) {
	transferLedgerEntries := []models.TransferLedgerEntry{}
	query := db.DB.Select(q.Eq("SentOrRead", false), q.Not(q.Eq("Action", REDOWNLOADFILEREQUEST)), q.Not(q.Eq("Action", DOWNLOADREQUEST)), q.Not(q.Eq("Action", UPLOADREQUEST)))
	err := query.Find(&transferLedgerEntries)
	if fmt.Sprintf("%s", err) == "not found" {
		return transferLedgerEntries, nil
	}
	if err != nil {
		return nil, err
	}
	return transferLedgerEntries, nil
}

// GetFileUploadRequests - get file upload requests
func (db *TransferLedger) GetFileUploadRequests(readCountLimit int) ([]models.TransferLedgerEntry, error) {
	transferLedgerEntries := []models.TransferLedgerEntry{}
	query := db.DB.Select(q.Eq("SentOrRead", false), q.Eq("Action", UPLOADREQUEST)).Limit(readCountLimit)
	err := query.Find(&transferLedgerEntries)
	if fmt.Sprintf("%s", err) == "not found" {
		return transferLedgerEntries, nil
	}
	if err != nil {
		return nil, err
	}
	return transferLedgerEntries, nil
}

// GetFileDownloadRequests - get file download requests
func (db *TransferLedger) GetFileDownloadRequests(readCountLimit int) ([]models.TransferLedgerEntry, error) {
	transferLedgerEntries := []models.TransferLedgerEntry{}
	actions := []string{}
	actions = append(actions, DOWNLOADREQUEST)
	actions = append(actions, REDOWNLOADFILEREQUEST)
	query := db.DB.Select(q.Eq("SentOrRead", false), q.In("Action", actions)).Limit(readCountLimit)
	err := query.Find(&transferLedgerEntries)
	if fmt.Sprintf("%s", err) == "not found" {
		return transferLedgerEntries, nil
	}
	if err != nil {
		return nil, err
	}
	return transferLedgerEntries, nil
}

// IsFileAlreadyDownloaded - update an existing entry to transfer ledger
func (db *TransferLedger) IsFileAlreadyDownloaded(fileID string) bool {
	transferLedgerEntries := []models.TransferLedgerEntry{}
	query := db.DB.Select(q.Eq("Action", "DOWNLOADED"+fileID), q.Eq("SentOrRead", true))
	query.Limit(2)
	err := query.Find(&transferLedgerEntries)
	if fmt.Sprintf("%s", err) == "not found" {
		return false
	}
	if err != nil {
		return false
	}
	if len(transferLedgerEntries) > 0 {
		return true
	}
	return false
}
