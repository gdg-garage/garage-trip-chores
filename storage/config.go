package storage

type Config struct {
	DbPath         string `mapstructure:"dbpath"`
	DiscordToken   string `mapstructure:"discordtoken"`
	PresentRole    string `mapstructure:"presentrole"`
	SkillPrefix    string `mapstructure:"skillprefix"`
	DiscordGuildId string `mapstructure:"discordguildid"`
}
