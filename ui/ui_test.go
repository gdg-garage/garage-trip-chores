package ui

import "testing"

func TestGetChoreIdFromButton(t *testing.T) {
	tests := []struct {
		name      string
		customID  string
		wantID    uint
		expectErr bool
	}{
		{
			name:      "valid custom ID",
			customID:  "schedule_button_click:123",
			wantID:    123,
			expectErr: false,
		},
		{
			name:      "valid custom ID with zero",
			customID:  "delete_button_click:0",
			wantID:    0,
			expectErr: false,
		},
		{
			name:      "invalid format - no colon",
			customID:  "invalid_id",
			wantID:    0,
			expectErr: true,
		},
		{
			name:      "invalid format - too many parts",
			customID:  "button:123:extra",
			wantID:    0,
			expectErr: true,
		},
		{
			name:      "invalid chore ID - not a number",
			customID:  "button:abc",
			wantID:    0,
			expectErr: true,
		},
		{
			name:      "invalid chore ID - empty",
			customID:  "button:",
			wantID:    0,
			expectErr: true,
		},
		{
			name:      "empty custom ID",
			customID:  "",
			wantID:    0,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := getChoreIdFromButton(tt.customID)

			if (err != nil) != tt.expectErr {
				t.Errorf("getChoreIdFromButton() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			if gotID != tt.wantID {
				t.Errorf("getChoreIdFromButton() gotID = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}
