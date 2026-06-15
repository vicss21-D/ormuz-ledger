package server

import (
	"time"
	
	"ormuz-ledger/pkg/model"
	"ormuz-ledger/pkg/routing"
)

// =========================================================
// 1. STATION <-> DRONE API (Comando e Controle - C2)
// =========================================================

// RegisterDroneRequest: Drone recém-nascido pede para ingressar na frota da Estação
type RegisterDroneRequest struct {
	DroneID string `json:"drone_id"`
	IP 		string `json:"ip,omitempty"`
}

// TelemetryRequest: Heartbeat contínuo do Drone para a Estação (Watchdog)
type TelemetryRequest struct {
	DroneID string  `json:"drone_id"`
	Status  string  `json:"status"` // "IDLE", "IN_FLIGHT", "RETURNING", "DESTROYED"
	Battery float64 `json:"battery"`
}

// DispatchMissionRequest: Estação envia a missão e sua própria localização para o Drone
type DispatchMissionRequest struct {
	Mission       model.Mission     `json:"mission"`
	StationAnchor routing.Coordinate  `json:"station_anchor"` // Usado pelo Drone para calcular o tempo de voo
}

// =========================================================
// 2. BROKER <-> STATION/DRONE API (Fila Tática e Lease)
// =========================================================

// ActionRequest: Usado para ACK, NACK e RENEW (Renovação do Lease)
// Pode ser enviado pela Estação ou diretamente pelo Drone (cenário de Estação destruída)
type ActionRequest struct {
	EventID string `json:"event_id"`
	DroneID string `json:"drone_id"` // Identifica o agente final da ação
	Reason  string `json:"reason,omitempty"`
}

// ActionResponse: Resposta padrão do Broker para operações de estado
type ActionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// ========== API Response Types ==========

// HealthCheckResponse resposta de health check
type HealthCheckResponse struct {
	Status    string    `json:"status"` // "ok" ou "degraded"
	Timestamp time.Time `json:"timestamp"`
}

// ErrorResponse resposta de erro padronizada
type ErrorResponse struct {
	Error     string    `json:"error"`
	Code      string    `json:"code"`
	Timestamp time.Time `json:"timestamp"`
}