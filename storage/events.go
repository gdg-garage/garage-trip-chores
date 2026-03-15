package storage

import "sync"

type EventType string

const (
	TaskCreated  EventType = "task_created"
	TaskUpdated  EventType = "task_updated"
	TaskAssigned EventType = "task_assigned"
	TaskAcked    EventType = "task_acked"
	TaskRefused  EventType = "task_refused"
	TaskTimeout  EventType = "task_timeout"
	TaskDone     EventType = "task_done"
)

type Event struct {
	Type       EventType        `json:"type"`
	Chore      *Chore           `json:"chore,omitempty"`
	Assignment *ChoreAssignment `json:"assignment,omitempty"`
}

type EventBus struct {
	listeners []chan Event
	mu        sync.RWMutex
}

func NewEventBus() *EventBus {
	return &EventBus{}
}

func (b *EventBus) Subscribe() chan Event {
	ch := make(chan Event, 100)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.listeners = append(b.listeners, ch)
	return ch
}

func (b *EventBus) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, listener := range b.listeners {
		if listener == ch {
			b.listeners = append(b.listeners[:i], b.listeners[i+1:]...)
			close(ch)
			return
		}
	}
}

func (b *EventBus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.listeners {
		select {
		case ch <- e:
		default:
		}
	}
}
