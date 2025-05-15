package config

import (
	"github.com/gdg-garage/garage-trip-chores/logger"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

type Config struct {
	Logger logger.Config
	Db     storage.Config
}

func defaultConf() *Config {
	return &Config{
		Logger: logger.Config{
			Level:       "debug",
			IncludeFile: true,
		},
		Db: storage.Config{
			DbPath: "data/db.sqlite",
		},
	}
}

func New() *Config {
	return defaultConf()
}
