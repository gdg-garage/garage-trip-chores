package main

import (
	"fmt"
	"os"

	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/config"
	"github.com/gdg-garage/garage-trip-chores/logger"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

func main() {
	conf, err := config.New()
	if err != nil {
		fmt.Println("Error reading config", "error", err)
		os.Exit(1)
	}
	logger := logger.New(conf.Logger)

	logger.Debug("Config loaded", "conf", conf)

	logger.Info("Chores!")

	logger.Debug("Initializing storage")
	s, err := storage.New(conf.Db, logger)
	if err != nil {
		logger.Error("Error initializing storage", "error", err)
		os.Exit(1)
	}
	logger.Debug("Storage initialized")

	logger.Debug("Adding a test chore")
	c, err := s.SaveChore(
		storage.Chore{
			Name:             "Test Chore",
			EstimatedTimeMin: 20,
			Creator:          "chores overlord",
			NecessaryWorkers: 1,
		})
	if err != nil {
		logger.Error("Error adding chore", "error", err)
		return
	}
	logger.Debug("Chore added", "chore", c)
	c.Complete()
	logger.Debug("Chore completed", "chore", c)
	logger.Debug("Updating chore")
	c, err = s.SaveChore(c)
	if err != nil {
		logger.Error("Error updating chore", "error", err)
		return
	}
	logger.Debug("Chore updated", "chore", c)
	completed, err := s.GetCompletedChores()
	if err != nil {
		logger.Error("Error getting completed chores", "error", err)
	} else {
		for _, c := range completed {
			logger.Debug("Completed chore", "chore", c.Name, "id", c.ID)
		}
	}

	// wl := storage.WorkLog{
	// 	Chore:        c,
	// 	TimeSpentMin: c.EstimatedTimeMin,
	// 	UserHandle:   "testuser2",
	// }
	// wl, err = s.SaveWorkLog(wl)
	// if err != nil {
	// 	logger.Error("Error saving work log", "error", err)
	// }
	// logger.Debug("Work log saved", "worklog", wl)

	st, err := s.GetUserStats()
	if err != nil {
		logger.Error("Error getting stats", "error", err)
	} else {
		for user, stats := range st {
			logger.Debug("User stats", "user", user, "stats", stats)
		}
	}

	// a, err := s.AssignChore(c, "testuser")
	// if err != nil {
	// 	logger.Error("Error assigning chore", "error", err)
	// } else {
	// 	logger.Debug("Chore assigned", "assignment", a)
	// }

	assignedStats, err := s.GetAssignedStats()
	if err != nil {
		logger.Error("Error getting assigned stats", "error", err)
	} else {
		for user, stats := range assignedStats {
			logger.Debug("User assigned stats", "user", user, "stats", stats)
		}
	}

	totalStats := st.Add(assignedStats)
	for user, stats := range totalStats {
		logger.Debug("User total stats", "user", user, "stats", stats)
	}

	cl := chores.NewChoresLogic(s, logger, conf.Chores)
	users, err := s.GetPresentUsers()
	if err != nil {
		logger.Error("Error getting present users", "error", err)
	} else {
		for _, user := range users {
			logger.Debug("Present user", "user", user.Handle, "capabilities", user.Capabilities)
		}
	}
	ass, err := cl.AssignChoresToUsers(users, c)
	if err != nil {
		logger.Error("Error assigning chores to users", "error", err)
	} else {
		logger.Debug("Chores assigned to users", "cnt", len(ass))
		for _, a := range ass {
			logger.Debug("Chore assigned to user", "assignment", a)
		}
	}

	ts, err := s.GetTotalChoreStats()
	if err != nil {
		logger.Error("Error getting total stats", "error", err)
	} else {
		for user, stats := range ts {
			logger.Debug("User total stats", "user", user, "stats", stats)
		}
	}

	s.GetPresentUsers()
}
