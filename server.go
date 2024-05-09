//go:build !cgo
// +build !cgo

package main

import (
	"flag"
	"io/fs"
	"net/http"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"

	"go.uber.org/zap"
)

var config Config

var config_path = flag.String("conf", "config.toml", "Path to config file")
var db_path = flag.String("db", "subscriptions.db", "Path to database file")
var article_db_path = flag.String("article-db", "articles.db", "Path to article database file")
var log_path = flag.String("log", "log.log", "Path to log file")
var save_log = flag.Bool("save-log", false, "Save log to file")
var verbose = flag.Bool("verbose", false, "Enable verbose logging")

var versionStr = "v0.0.7"

func main() {

	flag.Parse()

	if *verbose {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	setLogger(*log_path, *verbose, *save_log)
	defer logger.Sync()
	// Parse config file
	if _, err := toml.DecodeFile(*config_path, &config); err != nil {
		logger.Fatal("Failed to parse config file:", zap.Error(err))
		panic(err)
	}

	logger.Info("Telegram API URL: " + config.TelegramAPIURL)
	logger.Info("Post URL: " + config.PostURL)
	logger.Info("Gin Address: " + config.GinAddress)

	// Initialize the database and bot
	initDB()
	initBot(config.TelegramToken, config.TelegramAPIURL)
	initMarkdownRender()

	router := gin.Default()

	router.Use(loggerGinMiddleware())
	router.Use(enableCors())

	router.GET("/", handleIndex)
	router.GET("/version", handleVersion)
	router.GET("/changelog", handleChangeLog)
	router.NoRoute(handle404)

	assertsSys, err := fs.Sub(embed_fs, "asserts")
	if err != nil {
		logger.Fatal("Failed to get asserts sub file system", zap.Error(err))
		panic(err)
	}
	router.StaticFS("/asserts", http.FS(assertsSys))

	apiGroup := router.Group("/api")

	apiGroup.POST("/:uuid/json", handleJSON)
	apiGroup.GET("/:uuid/get", handleGet)
	apiGroup.POST("/:uuid/form", handleForm)
	apiGroup.POST("/:uuid/file", handleFile)

	articleGroup := router.Group("/html")
	articleGroup.GET("/:uuid", handleHTML)
	articleGroup.GET("/", handleExample)

	go startBot()

	router.Run(config.GinAddress) // listen and serve on configured address
}
