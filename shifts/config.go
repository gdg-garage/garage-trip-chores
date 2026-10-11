package shifts

import "time"

type Config struct {
	AdminRole                 string        `json:"admin_role"`
	FreezeHour                int           `json:"freeze_hour"`
	DefaultStandbyRatio       float64       `json:"default_standby_ratio"`
	MaxShiftsPerPersonPerDay  int           `json:"max_shifts_per_person_per_day"`
	Timezone                  string        `json:"timezone"`
	ReminderLeadTime          time.Duration `json:"reminder_lead_time"`
}

func DefaultConfig() Config {
	return Config{
		AdminRole:                "g::t::orgs",
		FreezeHour:               12,
		DefaultStandbyRatio:      0.10,
		MaxShiftsPerPersonPerDay: 2,
		Timezone:                 "Europe/Prague",
		ReminderLeadTime:         15 * time.Minute,
	}
}
