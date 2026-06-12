package domain

type Telemetry struct {
	EventID       	string `json:"event_id"` 		// ID único do evento de telemetria
	SensorID  		string `json:"sensor_id"` 		// ID do sensor que gerou a telemetria
	SectorID  		int    `json:"sector_id"` 		// ID do setor onde o sensor está localizado
	IsRequisition  	bool   `json:"is_requisition"` 	// Indica se a telemetria deverá ser tratada como uma requisição (true) ou um dado comum (false)
	IsCritical 		bool   `json:"is_critical"` 	// Indica se a telemetria é crítica (true) ou não (false)	
}

func NewTelemetry(eventID string, sensorID string, sectorID int, isRequisition bool, isCritical bool) Telemetry {
	return Telemetry{
		EventID:       	eventID,
		SensorID:  		sensorID,
		SectorID:  		sectorID,
		IsRequisition:  isRequisition,
		IsCritical: 	isCritical,
	}
}