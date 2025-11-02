package models

import (
	"time"

	"github.com/google/uuid"
)

type Location struct {
	ID        uuid.UUID `json:"id"`
	Building  string    `json:"building"`
	Cancelled bool      `json:"cancelled"`
	CreatedAt time.Time `json:"created_at"`
	Details   string    `json:"details"`
	Level     int32     `json:"level"`
	Row       string    `json:"row"`
	Section   string    `json:"section"`
	UpdatedAt time.Time `json:"updated_at"`
}

type LocationPositon struct {
	Building string `json:"building"`
	Level    int32  `json:"level"`
	Row      string `json:"row"`
	Section  string `json:"section"`
}
