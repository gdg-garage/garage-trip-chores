package web

type Config struct {
	DiscordClientId     string `json:"discord_client_id" mapstructure:"discordclientid"`
	DiscordClientSecret string `json:"discord_client_secret" mapstructure:"discordclientsecret"`
	DiscordCallbackUrl  string `json:"discord_callback_url" mapstructure:"discordcallbackurl"`
	DiscordPaidRole     string `json:"discord_paid_role" mapstructure:"discordpaidrole"`
	DiscordAdminRole    string `json:"discord_admin_role" mapstructure:"discordadminrole"`
	SessionSecret       string `json:"session_secret" mapstructure:"sessionsecret"`
	TabletPassword      string   `json:"tablet_password" mapstructure:"tabletpassword"`
	AuthRequired        bool     `json:"auth_required" mapstructure:"authrequired"`
	ChildrenCount       int      `json:"children_count" mapstructure:"childrencount"`
	ApiKeys             []string `json:"api_keys" mapstructure:"apikeys"`
}
