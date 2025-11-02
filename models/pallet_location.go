package models

import (
	"github.com/google/uuid"
)

type PalletLocation struct {
	ScannerID  uuid.UUID `json:"scanner_id"`
	PalletID   uuid.UUID `json:"pallet_id"`
	LocationID uuid.UUID `json:"location_id"`
}
