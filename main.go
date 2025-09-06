package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/config"
	"github.com/gdg-garage/garage-trip-chores/logger"
	presencetracker "github.com/gdg-garage/garage-trip-chores/presence_tracker"
	"github.com/gdg-garage/garage-trip-chores/reminders"
	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/gdg-garage/garage-trip-chores/ui"
)

func main() {
	conf, err := config.New()
	if err != nil {
		fmt.Println("Error reading config", "error", err)
		os.Exit(1)
	}
	logger := logger.New(conf.Logger)
	logger.Debug("Config loaded", "conf", conf)

	logger.Debug("Initializing storage")

	s, err := storage.New(conf.Db, logger)
	if err != nil {
		logger.Error("Error initializing storage", "error", err)
		os.Exit(1)
	}
	logger.Debug("Storage initialized")

	cl := chores.NewChoresLogic(s, logger, conf.Chores)

	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGALRM, os.Interrupt)

	ui := ui.NewUi(s, logger, &cl, s.GetDiscord(), conf.Ui)
	go ui.Commands(ctx, &wg)

	tracker := presencetracker.NewTracker(s, logger, conf.Tracker)
	go tracker.RunTracker(ctx, &wg)

	reminder := reminders.NewReminder(s, ui, &cl, logger, &conf.Reminder)
	go reminder.RunReminder(ctx, &wg)

	<-sc

	logger.Info("Shutting down...")
	cancel()
	wg.Wait()
	logger.Info("Shutdown complete")
}
