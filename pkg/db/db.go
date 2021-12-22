package db

import (
	log "github.com/sirupsen/logrus"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	types "github.com/Gimulator/metrico/pkg/types"
)

func NewDatabase(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		log.WithField("error", err).Error("Failed to initialize sqlite database")
		return nil, err
	}

	if err := db.AutoMigrate(&types.ResourceSnapshot{}); err != nil {
		log.WithField("error", err).Error("Failed to migrate models to database")
		return nil, err
	}

	return db, err
}

// InsertSnapshop will insert a snapshot record in db regardless of the snapshop already being present in the table.
func InsertSnapshot(db *gorm.DB, snapshot *types.ResourceSnapshot) error {
	result := db.Create(&snapshot)
	return result.Error
}

// CleanInsert will insert a snapshot record in db. If the snapshop already exists in the table, nothing will be inserted.
func CleanInsert(db *gorm.DB, snapshot *types.ResourceSnapshot) error {
	var count int64
	result := db.Model(&types.ResourceSnapshot{}).Where(&types.ResourceSnapshot{
		PodName:       snapshot.PodName,
		ContainerName: snapshot.ContainerName,
		Timestamp:     snapshot.Timestamp,
	}).Count(&count)
	if result.Error != nil {
		return result.Error
	}
	if count > 0 {
		// An identical snapshot already exists. Nothing needs to be inserted.
		return nil
	} else {
		return InsertSnapshot(db, snapshot)
	}
}

func RetrieveAll(db *gorm.DB) (*[]types.ResourceSnapshot, error) {
	var snapshots []types.ResourceSnapshot
	result := db.Find(&snapshots)
	if result.Error != nil {
		return nil, result.Error
	}
	return &snapshots, nil
}
