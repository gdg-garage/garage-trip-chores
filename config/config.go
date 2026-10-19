package config

import (
	"fmt"
	"strings"

	"github.com/gdg-garage/garage-trip-chores/api"
	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/llm"
	"github.com/gdg-garage/garage-trip-chores/logger"
	presencetracker "github.com/gdg-garage/garage-trip-chores/presence_tracker"
	"github.com/gdg-garage/garage-trip-chores/registration"
	"github.com/gdg-garage/garage-trip-chores/reminders"
	"github.com/gdg-garage/garage-trip-chores/shifts"
	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/gdg-garage/garage-trip-chores/ui"
	"github.com/gdg-garage/garage-trip-chores/web"
	"github.com/spf13/viper"
)

type Config struct {
	Logger       logger.Config
	Db           storage.Config
	Chores       chores.Config
	Shifts       shifts.Config
	Registration registration.Config
	Ui           ui.Config
	Tracker      presencetracker.Config
	Reminder     reminders.Config
	Api          api.Config
	LLM          llm.Config
	Web          web.Config
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
	viper.SetDefault("chores.cooldownmin", 30)

	viper.SetDefault("shifts.adminrole", "g::t::orgs")
	viper.SetDefault("shifts.freezehour", 12)
	viper.SetDefault("shifts.defaultstandbyratio", 0.10)
	viper.SetDefault("shifts.maxshiftsperpersonperday", 2)
	viper.SetDefault("shifts.timezone", "Europe/Prague")
	viper.SetDefault("shifts.reminderleadtime", "15m")
	viper.SetDefault("shifts.event", "")

	_ = viper.BindEnv("shifts.adminrole", "SHIFTS_ADMIN_ROLE", "CHORES_SHIFTS_ADMIN_ROLE", "CHORES_SHIFTS_ADMINROLE")
	_ = viper.BindEnv("shifts.freezehour", "SHIFTS_FREEZE_HOUR", "CHORES_SHIFTS_FREEZE_HOUR", "CHORES_SHIFTS_FREEZEHOUR")
	_ = viper.BindEnv("shifts.defaultstandbyratio", "SHIFTS_DEFAULT_STANDBY_RATIO", "CHORES_SHIFTS_DEFAULT_STANDBY_RATIO", "CHORES_SHIFTS_DEFAULTSTANDBYRATIO")
	_ = viper.BindEnv("shifts.maxshiftsperpersonperday", "SHIFTS_MAX_SHIFTS_PER_DAY", "CHORES_SHIFTS_MAX_SHIFTS_PER_DAY", "CHORES_SHIFTS_MAXSHIFTSPERPERSONPERDAY")
	_ = viper.BindEnv("shifts.timezone", "SHIFTS_TIMEZONE", "CHORES_SHIFTS_TIMEZONE")
	_ = viper.BindEnv("shifts.event", "SHIFTS_EVENT", "CHORES_SHIFTS_EVENT")

	viper.SetDefault("registration.apiurl", "")
	viper.SetDefault("registration.apikey", "")
	viper.SetDefault("registration.event", "")
	viper.SetDefault("registration.syncintervalmin", 15)

	_ = viper.BindEnv("registration.apiurl", "REGISTRATION_API_URL", "CHORES_REGISTRATION_API_URL", "CHORES_REGISTRATION_APIURL")
	_ = viper.BindEnv("registration.apikey", "REGISTRATION_API_KEY", "CHORES_REGISTRATION_API_KEY", "CHORES_REGISTRATION_APIKEY")
	_ = viper.BindEnv("registration.event", "REGISTRATION_EVENT", "CHORES_REGISTRATION_EVENT")
	_ = viper.BindEnv("registration.syncintervalmin", "REGISTRATION_SYNC_INTERVAL_MIN", "CHORES_REGISTRATION_SYNC_INTERVAL_MIN", "CHORES_REGISTRATION_SYNCINTERVALMIN")

	viper.SetDefault("ui.discordchannelid", "???")

	viper.SetDefault("tracker.sampleperiodmin", 10)

	viper.SetDefault("reminder.checkperiodseconds", 2)
	viper.SetDefault("reminder.reminderatio", 0.1)

	viper.SetDefault("api.port", 8080)
	viper.SetDefault("api.host", "0.0.0.0")
	viper.SetDefault("api.cors", true)
	viper.SetDefault("api.apikeys", []string{})

	viper.SetDefault("llm.apikey", "")
	viper.SetDefault("llm.model", "gemini-3.7-flash")
	viper.SetDefault("llm.discordchannelid", "")
	viper.SetDefault("llm.servicetier", "flex")

	viper.SetDefault("web.discordclientid", "")
	viper.SetDefault("web.discordclientsecret", "")
	viper.SetDefault("web.discordcallbackurl", "")
	viper.SetDefault("web.discordpaidrole", "")
	viper.SetDefault("web.discordadminrole", "g::t::orgs")
	viper.SetDefault("web.sessionsecret", "")
	viper.SetDefault("web.tabletpassword", "garagetrip")
	viper.SetDefault("web.authrequired", true)
	viper.SetDefault("web.childrencount", 5)

	_ = viper.BindEnv("web.discordclientid", "DISCORD_CLIENT_ID")
	_ = viper.BindEnv("web.discordclientsecret", "DISCORD_CLIENT_SECRET")
	_ = viper.BindEnv("web.discordcallbackurl", "DISCORD_CALLBACK_URL")
	_ = viper.BindEnv("web.discordpaidrole", "DISCORD_PAID_ROLE")
	_ = viper.BindEnv("web.discordadminrole", "DISCORD_ADMIN_ROLE")
	_ = viper.BindEnv("web.sessionsecret", "SESSION_SECRET")
	_ = viper.BindEnv("web.tabletpassword", "TABLET_PASSWORD")
	_ = viper.BindEnv("web.authrequired", "AUTH_REQUIRED")
	_ = viper.BindEnv("web.childrencount", "CHILDREN_COUNT")

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
