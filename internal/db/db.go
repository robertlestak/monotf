package db

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func initPostgres() error {
	l := log.WithFields(log.Fields{
		"pkg": "db",
		"fn":  "initPostgres",
	})
	l.Debug("start")
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PASS"),
	)
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		l.WithError(err).Error("failed to connect to database")
		return err
	}
	l.Debug("end")
	return nil
}

func initSqlite() error {
	l := log.WithFields(log.Fields{
		"pkg": "db",
		"fn":  "initSqlite",
	})
	l.Debug("start")
	var err error
	DB, err = gorm.Open(sqlite.Open(os.Getenv("DB_PATH")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		l.WithError(err).Error("failed to connect to database")
		return err
	}
	l.Debug("end")
	return nil
}

func Init() error {
	l := log.WithFields(log.Fields{
		"pkg": "db",
		"fn":  "Init",
	})
	l.Debug("start")
	driverName := os.Getenv("DB_DRIVER")
	switch driverName {
	case "postgres":
		return initPostgres()
	case "sqlite":
		return initSqlite()
	default:
		l.WithField("driver", driverName).Error("unsupported database driver")
		return fmt.Errorf("unsupported database driver: %s", driverName)
	}
}
