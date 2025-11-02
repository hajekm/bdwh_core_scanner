package models

import (
	"github.com/google/uuid"
)

type PalletLocation struct {
	ScannerID    uuid.UUID `json:"scanner_id"`
	PalletNo     uuid.UUID `json:"pallet_no"`
	LocationCode uuid.UUID `json:"location_code"`
}
