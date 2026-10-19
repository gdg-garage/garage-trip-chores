package storage

type AggregatedUserStats struct {
	WorkedCount     float64 `json:"worked_count"`
	WorkedMin       float64 `json:"worked_min"`
	AssignedMin     float64 `json:"assigned_min"`
	AssignedCount   float64 `json:"assigned_count"`
	ShiftCount      float64 `json:"shift_count"`
	ShiftStandbyMin float64 `json:"shift_standby_min"`
	TotalMin        float64 `json:"total_min"`
	TotalCount      float64 `json:"total_count"`
	PresentTicks    int     `json:"present_ticks"`
	NormalizedTotal float64 `json:"normalized_total"`
}

func (s *Storage) GetAggregatedStats() (map[string]AggregatedUserStats, error) {
	return s.GetAggregatedStatsForEvent(s.GetActiveEventCode())
}

func (s *Storage) GetAggregatedStatsForEvent(event string) (map[string]AggregatedUserStats, error) {
	usersStats := map[string]AggregatedUserStats{}

	userStats, err := s.GetUserStatsForEvent(event)
	if err != nil {
		return nil, err
	}
	for k, v := range userStats {
		st := usersStats[k]
		st.WorkedCount = v.Count
		st.WorkedMin = v.TotalMin
		usersStats[k] = st
	}

	assignedStats, err := s.GetAssignedStatsForEvent(event)
	if err != nil {
		return nil, err
	}
	for k, v := range assignedStats {
		st := usersStats[k]
		st.AssignedMin = v.TotalMin
		st.AssignedCount = v.Count
		usersStats[k] = st
	}

	shiftStats, err := s.GetShiftStandbyStatsForEvent(event)
	if err == nil {
		for k, v := range shiftStats {
			st := usersStats[k]
			st.ShiftCount = v.Count
			st.ShiftStandbyMin = v.TotalMin
			usersStats[k] = st
		}
	}

	totalStats, err := s.GetTotalChoreStatsForEvent(event)
	if err != nil {
		return nil, err
	}
	for k, v := range totalStats {
		st := usersStats[k]
		st.TotalMin = v.TotalMin
		st.TotalCount = v.Count
		usersStats[k] = st
	}

	usersPresenceCounts, err := s.GetUsersPresenceCountsForEvent(event)
	if err != nil {
		return nil, err
	}
	for k, v := range usersPresenceCounts {
		st := usersStats[k]
		st.PresentTicks = v
		usersStats[k] = st
	}

	normalizedStats, err := s.GetTotalNormalizedChoreStatsForEvent(event)
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
