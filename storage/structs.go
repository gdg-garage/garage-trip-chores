package storage

import "time"

type User struct {
	Handle       string
	Capabilities []string
}

type Chore struct {
	Name                  string
	NecessaryCapabilities []string
	NecessaryWorkers      []string
	EstimatedTime         int
	Completed             bool
	Deadline              *time.Time
}
