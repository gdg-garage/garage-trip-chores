package llm

type Config struct {
	ApiKey           string `mapstructure:"apikey"`
	Model            string `mapstructure:"model"`
	DiscordChannelId string `mapstructure:"discordchannelid"`
	ApiBaseUrl       string `mapstructure:"apibaseurl"`
}
