package ledgers

import (
	"ds-agent/models"
	"fmt"

	"github.com/asdine/storm"
	"github.com/asdine/storm/q"
)

const (
	//RUNNING - status when agent is successfully running
	RUNNING = "RUNNING"

	//STOPPED - status when agent is stopped
	STOPPED = "STOPPED"
)

type MonitorLedgerService interface {
	AddOrUpdateEntry(entry *models.MonitoringLedgerEntry) error
	GetAllEntries() ([]models.MonitoringLedgerEntry, error)
}

// MonitoringLedger - ledger to main flows creation/deletion
type MonitoringLedger struct {
	DB    *storm.DB
	STORE string
}

// InitMonitoringLedger - intialize ledger ledger
func InitMonitoringLedger(filePath string, store string) (*MonitoringLedger, error) {
	db, err := storm.Open(filePath)
	if err != nil {
		return nil, err
	}
	monitoringLedger := MonitoringLedger{
		DB: db,
	}
	return &monitoringLedger, nil
}

// AddOrUpdateEntry - add new entry to monitoring ledger
func (db *MonitoringLedger) AddOrUpdateEntry(entry *models.MonitoringLedgerEntry) error {
	var newEntry models.MonitoringLedgerEntry
	err := db.DB.One("AgentID", entry.AgentID, &newEntry)
	if fmt.Sprintf("%s", err) == "not found" {
		err = db.DB.Save(entry)
		if err != nil {
			return err
		}
	} else {
		if err != nil {
			return err
		}
		err = db.DB.Update(entry)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetAllEntries - get all entries of monitoring table
func (db *MonitoringLedger) GetAllEntries() ([]models.MonitoringLedgerEntry, error) {
	monitoringLedgerEntries := []models.MonitoringLedgerEntry{}
	query := db.DB.Select(q.Eq("Status", "RUNNING"))
	err := query.Find(&monitoringLedgerEntries)
	if fmt.Sprintf("%s", err) == "not found" {
		return monitoringLedgerEntries, nil
	}
	if err != nil {
		return nil, err
	}
	return monitoringLedgerEntries, nil
}
