package registration

type Config struct {
	ApiUrl          string `mapstructure:"apiurl" json:"api_url"`
	ApiKey          string `mapstructure:"apikey" json:"api_key"`
	Event           string `mapstructure:"event" json:"event"`
	SyncIntervalMin int    `mapstructure:"syncintervalmin" json:"sync_interval_min"`
}

func DefaultConfig() Config {
	return Config{
		ApiUrl:          "",
		ApiKey:          "",
		Event:           "",
		SyncIntervalMin: 15,
	}
}
