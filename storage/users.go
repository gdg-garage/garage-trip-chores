package storage

import "github.com/gdg-garage/garage-trip-chores/chores"

func AddUser(user User) error {
	return nil
}

func AddCapability(userId int, capability string) error {
	return nil
}

func RemoveCapability(userId int, capability string) error {
	return nil
}

func GetUsers() ([]User, error) {
	return nil, nil
}

func GetStats() (chores.ChoreStats, error) {
	return chores.ChoreStats{}, nil
}
