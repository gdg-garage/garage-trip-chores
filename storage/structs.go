package storage

import "time"

type User struct {
	Handle       string
	Capabilities []string
}

type Chore struct {
	Id                    int
	Name                  string
	NecessaryCapabilities []string
	NecessaryWorkers      []string
	EstimatedTimeMin      int
	Completed             bool
	Deadline              *time.Time
}

type WorkLog struct {
	Id           int
	UserHandle   string
	ChoreId      int
	TimeSpentMin *int
}

type ChoreAssignments struct {
	Id         int
	UserHandle string
	ChoreId    int
	Acked      bool
	Created    time.Time
	Refused    *time.Time
	Timeouted  *time.Time
}
