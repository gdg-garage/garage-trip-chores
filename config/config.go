package config

import (
	"fmt"
	"strings"

	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/logger"
	presencetracker "github.com/gdg-garage/garage-trip-chores/presence_tracker"
	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/gdg-garage/garage-trip-chores/ui"
	"github.com/spf13/viper"
)

type Config struct {
	Logger  logger.Config
	Db      storage.Config
	Chores  chores.Config
	Ui      ui.Config
	Tracker presencetracker.Config
}

func New() (*Config, error) {
	viper.SetDefault("logger.level", "debug")
	viper.SetDefault("logger.includefile", true)

	viper.SetDefault("db.dbpath", "data/db.sqlite")
	viper.SetDefault("db.discordtoken", "???")
	viper.SetDefault("db.discordguildid", "???")
	viper.SetDefault("db.presentrole", "chores::present")
	viper.SetDefault("db.skillprefix", "skill::")

	viper.SetDefault("chores.oversampleratio", 0.5)

	viper.SetDefault("ui.discordchannelid", "???")

	viper.SetDefault("tracker.sampleperiodmin", 10)

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	viper.SetEnvPrefix("CHORES")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Println("Config file not found, continuing without it.")
			err = nil
		} else {
			return nil, err
		}
	}

	config := Config{}
	err = viper.Unmarshal(&config)
	if err != nil {
		return nil, err
	}

	return &config, err
}
