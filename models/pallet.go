package models

import (
	"time"

	"github.com/google/uuid"
)

type Pallet struct {
	ID                  uuid.UUID `json:"id"`
	BoxCount            int32     `json:"box_count"`
	Cancelled           bool      `json:"cancelled"`
	CreatedAt           time.Time `json:"created_at"`
	InvoiceNo           string    `json:"invoice_no"`
	ItemsPerBox         int32     `json:"items_per_box"`
	OriginShipToAddress string    `json:"origin_ship_to_address"`
	OriginShipToCountry string    `json:"origin_ship_to_country"`
	PalletNo            string    `json:"pallet_no"`
	PartNo              string    `json:"part_no"`
	SapNo               string    `json:"sap_no"`
	Shipped             time.Time `json:"shipped"`
	StorageLocation     string    `json:"storage_location"`
	TotalCount          int32     `json:"total_count"`
	UnknownNo           string    `json:"unknown_no"`
	UpdatedAt           time.Time `json:"updated_at"`
}
