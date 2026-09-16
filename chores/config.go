package chores

type Config struct {
	OversampleRatio float64 `mapstructure:"oversampleratio"`
	CooldownMin     uint    `mapstructure:"cooldownmin"`
}
