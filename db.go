package main

import (
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var db *gorm.DB
var article_db *gorm.DB
var inMemoyrChatStore = make(map[int]int)

func initSpecialDB[T any](dbPath string) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		logger.Fatal("Failed to connect database: "+dbPath+" with error :", zap.Error(err))
		panic("failed to connect database:" + dbPath)
	}
	// Migrate the schema
	var t T
	db.AutoMigrate(&t)
	logger.Info("Database " + dbPath + " connection initialized")
	return db
}

func initDB() {
	db = initSpecialDB[Subscription](*db_path)
	article_db = initSpecialDB[Article](*article_db_path)
}
