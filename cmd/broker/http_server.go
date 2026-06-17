package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	"sync"

	"ormuz-ledger/pkg/queue"
	"ormuz-ledger/pkg/server"
	"ormuz-ledger/internal/inventory"
	"ormuz-ledger/pkg/model"
)

// PendingMission guarda a missão que foi entregue a uma Estação e o momento exato do checkout
type PendingMission struct {
	Mission      model.Mission
	CheckoutTime time.Time
}

// HTTPServer gerencia a API HTTP do Broker para C2 e Drones
type HTTPServer struct {
	Queue        *queue.PriorityQueue
	ShadowBuffer *ShadowBufferManager
	Unverified   *UnverifiedBufferManager
	Filter	   	 *cache.IdempotencyFilter

	LedgerClient *LedgerClient

	pendingMutex    sync.RWMutex
	pendingMissions map[string]PendingMission
}

// NewHTTPServer cria uma nova instância do servidor HTTP
func NewHTTPServer(mq *queue.PriorityQueue, sb *ShadowBufferManager, ub *UnverifiedBufferManager, lc *LedgerClient, filter *cache.IdempotencyFilter) *HTTPServer {
	server := &HTTPServer{
		Queue:        mq,
		ShadowBuffer: sb,
		Unverified:   ub,
		LedgerClient: lc,
		Filter:    	  filter,

		pendingMissions: make(map[string]PendingMission),
	}

	go server.startPendingJanitor()

	return server
}

// SetupRouter configura todas as rotas HTTP do servidor
func (hs *HTTPServer) SetupRouter() *http.ServeMux {
	mux := http.NewServeMux()

	// ========== HEALTH CHECK ==========
	mux.HandleFunc("/health", hs.healthCheck)

	// ========== STATION MANAGEMENT ========== 
	
	mux.HandleFunc("/queue/pop", hs.HandleQueuePop)
	mux.HandleFunc("/queue/resolve", hs.HandleQueueResolve)

	// ========== EXPLORER ==========
	mux.HandleFunc("/explorer", hs.handleExplorer)

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

// ========== STATION MANAGEMENT ==========

// startPendingJanitor varre o mapa de pendências e devolve à fila missões esquecidas
func (s *HTTPServer) startPendingJanitor() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		now := time.Now()
		s.pendingMutex.Lock()
		
		for eventID, pending := range s.pendingMissions {
			// Se passou 60 segundos e a Estação não respondeu, a missão volta à estaca zero
			if now.Sub(pending.CheckoutTime) > 60*time.Second {
				delete(s.pendingMissions, eventID)
				s.Queue.Enqueue(pending.Mission)
				log.Printf("[BROKER-JANITOR] ⚠️ Missão %s sofreu TIMEOUT absoluto. Devolvida à Fila Heap.", eventID[:8])
			}
		}
		
		s.pendingMutex.Unlock()
	}
}

func (s *HTTPServer) HandleQueuePop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mission, found := s.Queue.Dequeue()
	if !found {
		w.WriteHeader(http.StatusNoContent) // Fila vazia, a Estação deve aguardar
		return
	}

	eventID := mission.Payload.EventID

	// Regista o Checkout
	s.pendingMutex.Lock()
	s.pendingMissions[eventID] = PendingMission{
		Mission:      mission,
		CheckoutTime: time.Now(),
	}
	s.pendingMutex.Unlock()

	log.Printf("[BROKER] Checkout da Missão %s para C2 (Timeout em 60s)", eventID[:8])

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mission)
}

// HandleQueueResolve: A Estação devolve o resultado (Falha de comunicação do Drone ou Sucesso absoluto)
func (s *HTTPServer) HandleQueueResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Action  string        `json:"action"` // "SUCCESS" ou "REQUEUE"
		Mission model.Mission `json:"mission"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	eventID := payload.Mission.Payload.EventID

	// Tenta resolver e remover a pendência
	s.pendingMutex.Lock()
	_, exists := s.pendingMissions[eventID]
	if exists {
		delete(s.pendingMissions, eventID)
	}
	s.pendingMutex.Unlock()

	// Se não existe, é porque já sofreu timeout no Broker (proteção contra "fantasmas" da rede)
	if !exists {
		log.Printf("[BROKER] Sinal rejeitado: Missão %s já não estava pendente (Timeout ou Resolvida).", eventID[:8])
		w.WriteHeader(http.StatusOK)
		return
	}

	if payload.Action == "REQUEUE" {
		// A Estação percebeu a queda do Drone antes do timeout de 60s. Devolvemos à fila.
		s.Queue.Enqueue(payload.Mission)
		log.Printf("[BROKER] Missão %s devolvida à fila ativamente pela Estação", eventID[:8])
	} else if payload.Action == "SUCCESS" {
		log.Printf("[BROKER] Estação confirmou sucesso da Missão %s", eventID[:8])
	}

	w.WriteHeader(http.StatusOK)
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

// ========== EXPLORER ==========

// handleExplorer expõe o estado global do Ledger para entidades externas (C2, Analistas, etc)
func (s *HTTPServer) handleExplorer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	stateJSON, err := s.LedgerClient.QueryGlobalState()
	if err != nil {
		http.Error(w, fmt.Sprintf("Falha ao ler a Blockchain: %v", err), http.StatusInternalServerError)
		return
	}

	// Devolve o JSON formatado e legível para a entidade
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(stateJSON)
}
