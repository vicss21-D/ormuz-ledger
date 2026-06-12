package domain

type Telemetry struct {
	EventID       string `json:"event_id"`       // ID único do evento de telemetria
	SensorID      string `json:"sensor_id"`      // ID do sensor que gerou a telemetria
	SectorID      int    `json:"sector_id"`      // ID do setor onde o sensor está localizado
	IsRequisition bool   `json:"is_requisition"` // Indica se a telemetria deverá ser tratada como requisição
	IsCritical    bool   `json:"is_critical"`    // Indica se a telemetria é crítica
	
	// CAMPOS PARA INTEGRAÇÃO BLOCKCHAIN
	NationID      string `json:"nation_id"`      // Identifica a Nação/Entidade dona do setor que pagará o crédito
	Signature     string `json:"signature"`      // Assinatura criptográfica que prova a autenticidade da requisição
}

func NewTelemetry(eventID string, sensorID string, sectorID int, isRequisition bool, isCritical bool, nationID string, signature string) Telemetry {
	return Telemetry{
		EventID:       eventID,
		SensorID:      sensorID,
		SectorID:      sectorID,
		IsRequisition: isRequisition,
		IsCritical:    isCritical,
		NationID:      nationID,
		Signature:     signature,
	}
}