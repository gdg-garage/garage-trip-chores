package storage

import "github.com/gdg-garage/garage-trip-chores/chores"

func AddUser(user User) error {}

func AddCapability(userId int, capability string) error {}

func RemoveCapability(userId int, capability string) error {}

func GetUsers() ([]User, error) {}

func GetStats() (chores.ChoreStats, error) {}
