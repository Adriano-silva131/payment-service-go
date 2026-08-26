package domain

import (
	"time"

	"github.com/google/uuid"
)

type DltStatus string

const (
	DltStatusPending     DltStatus = "PENDING"
	DltStatusReprocessed DltStatus = "REPROCESSED"
)

type DltMessage struct {
	ID            uuid.UUID
	OriginalTopic string
	MessageKey    string
	EventType     string
	Payload       string
	ErrorMessage  string
	Status        DltStatus
	CreatedAt     time.Time
	ReprocessedAt *time.Time
}
