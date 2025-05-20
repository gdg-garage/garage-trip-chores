package storage

import (
	"slices"
	"testing"
)

func TestChoreCapabilities(t *testing.T) {
	c := Chore{
		Name: "Test Chore"}

	if len(c.necessaryCapabilities) > 0 {
		t.Error("Expected empty capabilities list")
	}
	if c.NecessaryCapabilities != "" {
		t.Error("Expected empty capabilities string")
	}
	c.SetCapabilities([]string{"cap1", "cap2"})
	if len(c.necessaryCapabilities) != 2 {
		t.Error("Expected 2 capabilities")
	}
	if c.NecessaryCapabilities != "cap1,cap2" {
		t.Error("Expected capabilities string to be 'cap1,cap2'")
	}
	if !slices.Equal(c.GetCapabilities(), []string{"cap1", "cap2"}) {
		t.Error("Expected capabilities to be 'cap1,cap2'")
	}
}
