package logger

type Config struct {
	Level       string
	IncludeFile bool `mapstructure:"includefile"`
}
