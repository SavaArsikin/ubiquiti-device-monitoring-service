package rest

import (
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
)

type deviceInput struct {
	Name      string `json:"name"`
	HealthURL string `json:"health_url"`
}

type deviceResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	HealthURL  string     `json:"health_url"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ArchivedAt *time.Time `json:"archived_at"`
}

func (input deviceInput) toDevice() *device.Device {
	return &device.Device{
		Name:      input.Name,
		HealthURL: input.HealthURL,
	}
}
