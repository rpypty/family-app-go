package logger

import (
	"log"
	"os"
)

func New() *log.Logger {
	return log.New(os.Stdout, "", log.LstdFlags|log.LUTC)
}
