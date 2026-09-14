package ui

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

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
			gotID, err := getChoreIdFromCustomID(tt.customID)

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

func TestGetInteractionUserId(t *testing.T) {
	tests := []struct {
		name     string
		input    *discordgo.InteractionCreate
		expected string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: "",
		},
		{
			name:     "empty interaction",
			input:    &discordgo.InteractionCreate{},
			expected: "",
		},
		{
			name: "from interaction user",
			input: &discordgo.InteractionCreate{
				Interaction: &discordgo.Interaction{
					User: &discordgo.User{ID: "user_123"},
				},
			},
			expected: "user_123",
		},
		{
			name: "from interaction member user",
			input: &discordgo.InteractionCreate{
				Interaction: &discordgo.Interaction{
					Member: &discordgo.Member{
						User: &discordgo.User{ID: "member_456"},
					},
				},
			},
			expected: "member_456",
		},
		{
			name: "member without user",
			input: &discordgo.InteractionCreate{
				Interaction: &discordgo.Interaction{
					Member: &discordgo.Member{},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getInteractionUserId(tt.input)
			if got != tt.expected {
				t.Errorf("getInteractionUserId() = %q, want %q", got, tt.expected)
			}
		})
	}
}
