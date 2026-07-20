package main

import (
	"os"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/ccswitch/internal/infrastructure/controllers"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	logger.SetFormatter(&logger.TextFormatter{
		FullTimestamp: true,
	})
	if os.Getenv("DEBUG") == "true" {
		logger.SetLevel(logger.DebugLevel)
	}

	if err := controllers.NewRootCommand(version).Execute(); err != nil {
		os.Exit(1)
	}
}
