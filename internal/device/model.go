package device

import (
	"errors"
	"time"
)

var ErrDeviceNotFound = errors.New("device not found")

type Device struct {
	ID         string
	Name       string
	HealthURL  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ArchivedAt *time.Time
}
