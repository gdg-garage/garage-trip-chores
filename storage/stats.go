package storage

type AggregatedUserStats struct {
	WorkedCount     float64 `json:"worked_count"`
	WorkedMin       float64 `json:"worked_min"`
	AssignedMin     float64 `json:"assigned_min"`
	AssignedCount   float64 `json:"assigned_count"`
	TotalMin        float64 `json:"total_min"`
	TotalCount      float64 `json:"total_count"`
	PresentTicks    int     `json:"present_ticks"`
	NormalizedTotal float64 `json:"normalized_total"`
}

func (s *Storage) GetAggregatedStats() (map[string]AggregatedUserStats, error) {
	usersStats := map[string]AggregatedUserStats{}

	userStats, err := s.GetUserStats()
	if err != nil {
		return nil, err
	}
	for k, v := range userStats {
		st := usersStats[k]
		st.WorkedCount = v.Count
		st.WorkedMin = v.TotalMin
		usersStats[k] = st
	}

	assignedStats, err := s.GetAssignedStats()
	if err != nil {
		return nil, err
	}
	for k, v := range assignedStats {
		st := usersStats[k]
		st.AssignedMin = v.TotalMin
		st.AssignedCount = v.Count
		usersStats[k] = st
	}

	totalStats, err := s.GetTotalChoreStats()
	if err != nil {
		return nil, err
	}
	for k, v := range totalStats {
		st := usersStats[k]
		st.TotalMin = v.TotalMin
		st.TotalCount = v.Count
		usersStats[k] = st
	}

	usersPresenceCounts, err := s.GetUsersPresenceCounts()
	if err != nil {
		return nil, err
	}
	for k, v := range usersPresenceCounts {
		st := usersStats[k]
		st.PresentTicks = v
		usersStats[k] = st
	}

	normalizedStats, err := s.GetTotalNormalizedChoreStats()
	if err != nil {
		return nil, err
	}
	for k, v := range normalizedStats {
		st := usersStats[k]
		st.NormalizedTotal = v.TotalMin
		usersStats[k] = st
	}

	return usersStats, nil
}
