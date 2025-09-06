package reminders

type Config struct {
	CheckPeriodSeconds int     `mapstructure:"checkperiodseconds"`
	ReminderRatio      float64 `mapstructure:"reminderatio"`
}
