package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"ormuz-ledger/pkg/queue"
	//"ormuz-ledger/pkg/radar"
	"ormuz-ledger/pkg/server"
	"ormuz-ledger/internal/inventory"
	"ormuz-ledger/pkg/model"
)

// HTTPServer gerencia a API HTTP do Broker para C2 e Drones
type HTTPServer struct {
	Queue        *queue.PriorityQueue
	ShadowBuffer *ShadowBufferManager
	Unverified   *UnverifiedBufferManager
	Filter	   	 *cache.IdempotencyFilter
}

// NewHTTPServer cria uma nova instância do servidor HTTP
func NewHTTPServer(mq *queue.PriorityQueue, sb *ShadowBufferManager, ub *UnverifiedBufferManager, filter *cache.IdempotencyFilter) *HTTPServer {
	return &HTTPServer{
		Queue:        mq,
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

	// ========== STATION MANAGEMENT ========== 
	// future routes
	//mux.HandleFunc("/queue/pop", hs.HandleQueuePop)
	//mux.HandleFunc("/queue/resolve", hs.HandleQueueResolve)

	// ========== INTERNAL API (BROKER <-> BROKER GOSSIP) ==========
	mux.HandleFunc("/internal/shadow/sync", hs.HandleShadowSync)
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

	s.Filter.CheckAndAdd(eventID)

	switch payload.Action {
	case "ACK":
		// 2. Removemos da sala de espera (se o pacote UDP já tiver chegado)
		s.Unverified.RetrieveAndRemove(eventID)
		
		// 3. INCONDICIONAL: Se o Primário validou, a missão é garantida. 
		// Colocamos no Backup Oficial imediatamente!
		s.ShadowBuffer.Store(payload.Mission)
		log.Printf("[GOSSIP] Missão %s PROMOVIDA para o Shadow Buffer.", eventID[:8])

	case "NACK":
		// Primário rejeitou a cobrança. Limpamos a memória.
		if _, found := s.Unverified.RetrieveAndRemove(eventID); found {
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
