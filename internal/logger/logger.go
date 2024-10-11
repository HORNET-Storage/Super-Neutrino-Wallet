package logger

import (
	"log"
	"os"
)

var (
	InfoLogger  *log.Logger
	ErrorLogger *log.Logger
	logFile     *os.File
)

// Init initializes the loggers and creates/opens the log file
func Init(logFilePath string) error {
	var err error
	logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}

	InfoLogger = log.New(logFile, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLogger = log.New(logFile, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
	return nil
}

// RotateLog clears the current log file or creates a new one to start fresh
func RotateLog(logFilePath string) error {
	if logFile != nil {
		logFile.Close() // Close the current log file before rotating
	}

	// Reinitialize the log file with truncation to clear its contents
	var err error
	logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}

	// Reset the outputs for the loggers to the new log file
	InfoLogger.SetOutput(logFile)
	ErrorLogger.SetOutput(logFile)

	return nil
}

// Cleanup closes the log file when the application is done using it
func Cleanup() {
	if logFile != nil {
		logFile.Close()
	}
}

// Info logs an informational message to the log file
func Info(v ...interface{}) {
	InfoLogger.Println(v...)
}

// Error logs an error message to the log file
func Error(v ...interface{}) {
	ErrorLogger.Println(v...)
}
