package storage

type Config struct {
	DbPath         string `mapstructure:"dbpath"`
	DiscordToken   string `mapstructure:"discordtoken"`
	DiscordGuildId string `mapstructure:"discordguildid"`
	PresentRole    string `mapstructure:"presentrole"`
	SkillPrefix    string `mapstructure:"skillprefix"`
}
