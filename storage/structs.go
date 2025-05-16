package storage

import "time"

type User struct {
	Id           int
	Handle       string
	Capabilities []string
}

type Chore struct {
	Id                    int
	Name                  string
	NecessaryCapabilities []string
	NecessaryWorkers      []string
	EstimatedTime         int
	Completed             bool
	Deadline              *time.Time
}

type WorkLog struct {
	Id      int
	UserId  int
	ChoreId int
}
