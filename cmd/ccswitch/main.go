package main

import (
	"os"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/ccswitch/internal/infrastructure/controllers"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	// Colors are left to logrus, which turns them on for a terminal only: the
	// monitor daemon logs to a file, and forced colors would fill it with escape
	// codes.
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
