package model

import (
	"time"
)

type SensorData struct {
	EventID       string `json:"event_id"`
	SensorID      string `json:"sensor_id"`
	SectorID      int    `json:"sector_id"`
	IsRequisition bool   `json:"is_requisition"`
	IsCritical    bool   `json:"is_critical"`
	InternalClock uint64 `json:"internal_clock"`

	// INTEGRAÇÃO COM O LEDGER
	NationID      string `json:"nation_id"`
	Signature     string `json:"signature"`
}

type Mission struct {
	Payload      SensorData
	LamportClock uint64

	LeaseExpires time.Time `json:"lease_expires,omitempty"`
	LeaseHolder  string    `json:"lease_holder,omitempty"`
	BrokerAnchor string    `json:"broker_anchor"`
}

type RescueEvent struct {
	EventID  string
	FullScan bool
}