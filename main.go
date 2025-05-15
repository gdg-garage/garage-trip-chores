package main

import (
	"github.com/gdg-garage/garage-trip-chores/config"
	"github.com/gdg-garage/garage-trip-chores/logger"
)

func main() {
	// TODO: Read config from Cobra flags.
	conf := config.New()
	logger := logger.New(conf.Logger)

	logger.Info("Chores!")

}
