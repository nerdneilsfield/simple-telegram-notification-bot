package main

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

func setLogger(log_path string, debug bool, save_log bool) {
	var loggerConfig zap.Config
	if debug {
		loggerConfig = zap.NewDevelopmentConfig()
	} else {
		loggerConfig = zap.NewProductionConfig()
	}
	loggerConfig.EncoderConfig.TimeKey = "time"
	loggerConfig.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	var atom zap.AtomicLevel
	if debug {
		atom = zap.NewAtomicLevelAt(zap.DebugLevel)
	} else {
		atom = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	// 创建一个写入文件的 logger

	// 创建一个写入控制台的 logger
	consoleEncoder := zapcore.NewConsoleEncoder(loggerConfig.EncoderConfig)
	consoleCore := zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), atom)

	if save_log {
		fileEncoder := zapcore.NewJSONEncoder(loggerConfig.EncoderConfig)
		file, _ := os.Create("zap.log")
		fileCore := zapcore.NewCore(fileEncoder, zapcore.AddSync(file), atom)

		// 使用 zapcore.NewTee 合并 fileCore 和 consoleCore
		teeCore := zapcore.NewTee(fileCore, consoleCore)

		// 创建 logger
		logger = zap.New(teeCore, zap.AddCaller())
	} else {
		logger = zap.New(consoleCore, zap.AddCaller())
	}
}
