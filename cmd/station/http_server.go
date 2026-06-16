package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"ormuz-ledger/pkg/model"
	"ormuz-ledger/pkg/radar"
	"ormuz-ledger/pkg/server"
)

type StationServer struct {
	BrokerURL        string
	Radar            *radar.InFlightManager
	HTTPClient       *server.HTTPClient
	dronesMutex      sync.RWMutex
	RegisteredDrones map[string]time.Time
}

func NewStationServer(brokerURL string, flightRadar *radar.InFlightManager) *StationServer {
	return &StationServer{
		BrokerURL:        brokerURL,
		Radar:            flightRadar,
		HTTPClient:       server.NewHTTPClient(2 * time.Second),
		RegisteredDrones: make(map[string]time.Time),
	}
}

func (s *StationServer) SetupRouter() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/api/drone/register", s.handleRegister)
	mux.HandleFunc("/api/mission/pull", s.handleMissionPull)
	mux.HandleFunc("/api/mission/renew", s.handleMissionRenew)
	mux.HandleFunc("/api/mission/ack", s.handleMissionAck)
	return mux
}

func (s *StationServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		DroneID string `json:"drone_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	s.dronesMutex.Lock()
	s.RegisteredDrones[payload.DroneID] = time.Now()
	s.dronesMutex.Unlock()

	// DESCOBERTA DE SERVIÇO: Captura o Hostname/IP deste contentor no Swarm
	hostname, _ := os.Hostname()
	directURL := fmt.Sprintf("http://%s:8081", hostname)

	log.Printf("[C2] 🚁 Drone %s registado. (Sessão fixada em: %s)", payload.DroneID, directURL)
	
	// Devolve a Rota Direta para o Drone contornar o balanceador nas próximas chamadas
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"direct_url": directURL,
	})
}

func (s *StationServer) handleMissionPull(w http.ResponseWriter, r *http.Request) {
	droneID := r.URL.Query().Get("drone_id")
	
	s.dronesMutex.RLock()
	_, isRegistered := s.RegisteredDrones[droneID]
	s.dronesMutex.RUnlock()

	if !isRegistered {
		http.Error(w, "Drone não registado", http.StatusUnauthorized)
		return
	}

	var mission model.Mission
	url := fmt.Sprintf("%s/queue/pop", s.BrokerURL)
	err := s.HTTPClient.GetJSON(url, &mission)
	
	if err != nil || mission.Payload.EventID == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.Radar.MarkInFlight(mission, 10*time.Second)
	log.Printf("[C2] Missão %s delegada ao Drone %s", mission.Payload.EventID[:8], droneID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mission)
}

func (s *StationServer) handleMissionRenew(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("event_id")
	if s.Radar.RenewLease(eventID, 10*time.Second) {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusGone)
	}
}

func (s *StationServer) handleMissionAck(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		EventID string `json:"event_id"`
		DroneID string `json:"drone_id"`
	}
	json.NewDecoder(r.Body).Decode(&payload)

	s.Radar.Acknowledge(payload.EventID)

	resolveURL := fmt.Sprintf("%s/queue/resolve", s.BrokerURL)
	brokerPayload := map[string]interface{}{
		"action":  "SUCCESS",
		"mission": model.Mission{Payload: model.SensorData{EventID: payload.EventID}},
	}
	_ = s.HTTPClient.PostJSON(resolveURL, brokerPayload, nil)

	log.Printf("[C2] ✅ Missão %s reportada como concluída pelo Drone %s", payload.EventID[:8], payload.DroneID)
	w.WriteHeader(http.StatusOK)
}