package config

import (
	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/logger"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

type Config struct {
	Logger logger.Config
	Db     storage.Config
	Chores chores.Config
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
		Chores: chores.Config{
			OversampleRatio: 0.5,
		},
	}
}

func New() *Config {
	return defaultConf()
}
