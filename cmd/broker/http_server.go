package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"ormuz-ledger/pkg/queue"
	"ormuz-ledger/pkg/radar"
	"ormuz-ledger/pkg/server"
	"ormuz-ledger/internal/inventory"
)

// HTTPServer gerencia a API HTTP do Broker para C2 e Drones
type HTTPServer struct {
	Queue        *queue.PriorityQueue
	InFlight     *radar.InFlightManager
	ShadowBuffer *ShadowBufferManager
	Unverified   *UnverifiedBufferManager
	Filter	   	 *cache.IdempotencyFilter
}

// NewHTTPServer cria uma nova instância do servidor HTTP
func NewHTTPServer(mq *queue.PriorityQueue, ifm *radar.InFlightManager, sb *ShadowBufferManager, ub *UnverifiedBufferManager, filter *cache.IdempotencyFilter) *HTTPServer {
	return &HTTPServer{
		Queue:        mq,
		InFlight:     ifm,
		ShadowBuffer: sb,
		Unverified:   ub,
		Filter:    	  filter,
	}
}

// SetupRouter configura todas as rotas HTTP do servidor
func (hs *HTTPServer) SetupRouter() *http.ServeMux {
	mux := http.NewServeMux()

	// ========== HEALTH CHECK ==========
	mux.HandleFunc("/health", hs.healthCheck)

	// ========== DRONE API (C2 <-> DRONE) ==========
	mux.HandleFunc("/api/drone/register", hs.droneRegister)
	mux.HandleFunc("/api/drone/telemetry", hs.droneTelemetry)
	mux.HandleFunc("/api/mission/dispatch", hs.missionDispatch)

	// ========== MISSION STATE MANAGEMENT ==========
	mux.HandleFunc("/api/mission/acknowledge", hs.missionAck)
	mux.HandleFunc("/api/mission/nack", hs.missionNack)
	mux.HandleFunc("/api/mission/renew", hs.missionRenew)

	// ========== INTERNAL API (BROKER <-> BROKER GOSSIP) ==========
	mux.HandleFunc("/internal/shadow/sync", hs.shadowSync)
	mux.HandleFunc("/internal/topology/sync", hs.topologySync)

	return mux
}

// ========== HEALTH CHECK ==========

// healthCheck respondeu o status de saúde do broker
func (hs *HTTPServer) healthCheck(w http.ResponseWriter, r *http.Request) {
	resp := server.HealthCheckResponse{
		Status:    "ok",
		Timestamp: time.Now(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
	log.Printf("[HTTP] Health check: OK")
}

// ========== DRONE MANAGEMENT ==========

// droneRegister registra um novo drone na frota
func (hs *HTTPServer) droneRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req server.RegisterDroneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// TODO: Integrar com gerenciador de Drones
	resp := server.ActionResponse{
		Success: true,
		Message: fmt.Sprintf("Drone %s registrado com sucesso", req.DroneID),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
	log.Printf("[HTTP] Drone registrado: %s", req.DroneID)
}

// droneTelemetry recebe dados de telemetria do drone
func (hs *HTTPServer) droneTelemetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req server.TelemetryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// TODO: Processar telemetria
	resp := server.ActionResponse{
		Success: true,
		Message: "Telemetria recebida",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
	log.Printf("[HTTP] Telemetria do drone %s: Status=%s, Battery=%.1f%%", req.DroneID, req.Status, req.Battery)
}

// ========== MISSION DISPATCH & STATE ==========

// missionDispatch envia uma missão para o drone via estação
func (hs *HTTPServer) missionDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req server.DispatchMissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Marca a missão como em voo (In-Flight)
	hs.InFlight.MarkInFlight(req.Mission, 5*time.Minute)

	resp := server.ActionResponse{
		Success: true,
		Message: fmt.Sprintf("Missão %s despachada", req.Mission.Payload.EventID),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
	log.Printf("[HTTP] Missão despachada: %s para setor %d", req.Mission.Payload.EventID[:8], req.Mission.Payload.SectorID)
}

// missionAck confirma que o drone completou a missão
func (hs *HTTPServer) missionAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req server.ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Remove da memória de em-voo
	success := hs.InFlight.Acknowledge(req.EventID)

	resp := server.ActionResponse{
		Success: success,
		Message: fmt.Sprintf("ACK para missão %s", req.EventID[:8]),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
	log.Printf("[HTTP] Missão confirmada: %s (Drone: %s)", req.EventID[:8], req.DroneID)
}

// missionNack nega que o drone falhou na missão
func (hs *HTTPServer) missionNack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req server.ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Devolve para a fila
	success := hs.InFlight.Nack(req.EventID)

	resp := server.ActionResponse{
		Success: success,
		Message: fmt.Sprintf("NACK para missão %s: %s", req.EventID[:8], req.Reason),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
	log.Printf("[HTTP] Missão rejeitada: %s (Drone: %s) - Motivo: %s", req.EventID[:8], req.DroneID, req.Reason)
}

// missionRenew renova o lease de uma missão em voo
func (hs *HTTPServer) missionRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req server.ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Estende o lease por mais 5 minutos
	success := hs.InFlight.RenewLease(req.EventID, 5*time.Minute)

	resp := server.ActionResponse{
		Success: success,
		Message: fmt.Sprintf("Lease renovado para missão %s", req.EventID[:8]),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
	log.Printf("[HTTP] Lease renovado: %s (Drone: %s)", req.EventID[:8], req.DroneID)
}

// ========== INTERNAL BROKER API (GOSSIP) ==========

// shadowSync sincroniza as decisões de validação entre brokers
func (s *HTTPServer) HandleShadowSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Atualizamos a Struct para extrair o EventID da raiz
	var payload struct {
		Action  string        `json:"action"`
		EventID string        `json:"event_id"`
		Mission model.Mission `json:"mission"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	eventID := payload.EventID
	if eventID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch payload.Action {
	case "ACK":
		// 2. Removemos da sala de espera (se o pacote UDP já tiver chegado)
		s.UnverifiedBuffer.RetrieveAndRemove(eventID)
		
		// 3. INCONDICIONAL: Se o Primário validou, a missão é garantida. 
		// Colocamos no Backup Oficial imediatamente!
		s.ShadowBuffer.StoreOrphanedMission(payload.Mission)
		log.Printf("[GOSSIP] Missão %s PROMOVIDA para o Shadow Buffer.", eventID[:8])

	case "NACK":
		// Primário rejeitou a cobrança. Limpamos a memória.
		if _, found := s.UnverifiedBuffer.RetrieveAndRemove(eventID); found {
			log.Printf("[GOSSIP] Missão %s DESCARTADA (Rejeição Ledger).", eventID[:8])
		}
	}

	w.WriteHeader(http.StatusOK)
}

// topologySync sincroniza mudanças na topologia de brokers
func (hs *HTTPServer) topologySync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	payload := map[string]interface{}{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	log.Printf("[GOSSIP] Topologia sincronizada: %v", payload)

	// TODO: Processar mudanças na topologia

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"received": true})
}
