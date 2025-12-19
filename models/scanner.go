package models

import "github.com/google/uuid"

type ScannerInfo struct {
	ID          uuid.UUID `json:"id"`
	Alias       string    `json:"alias"`
	Paths       []string  `json:"paths"`
	VendorID    uint16    `json:"vendor_id"`
	ProductID   uint16    `json:"product_id"`
	BusType     string    `json:"bus_type"`
	ProductName string    `json:"product_name"`
	Serial      string    `json:"serial"`
}
