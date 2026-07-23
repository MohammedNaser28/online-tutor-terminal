package main

import (
	"log"
)

var logLevel string

func initLogLevel(level string) {
	logLevel = level
}

func logDebug(format string, v ...any) {
	if logLevel == "debug" {
		log.Printf(format, v...)
	}
}

func logInfo(format string, v ...any) {
	if logLevel != "error" && logLevel != "warn" {
		log.Printf(format, v...)
	}
}

func logWarn(format string, v ...any) {
	if logLevel != "error" {
		log.Printf(format, v...)
	}
}

func logError(format string, v ...any) {
	log.Printf(format, v...)
}
