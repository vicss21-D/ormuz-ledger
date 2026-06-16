package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"ormuz-ledger/pkg/model"
	"ormuz-ledger/pkg/server"
)

type LedgerTransaction struct {
	Type      string `json:"type"`
	NationID  string `json:"nation_id"`
	EventID   string `json:"event_id"`
	Signature string `json:"signature"`
	Payload   string `json:"payload"`
}

type Drone struct {
	ID          string
	StationURL  string
	CometURL    string
	HTTPClient  *server.HTTPClient
	IsRegistered bool
}

func main() {
	
	droneID := os.Getenv("DRONE_ID")
	if droneID == "" {
		droneID = fmt.Sprintf("UAV-%d", rand.Intn(9000)+1000)
	}

	// tasks.station resolve para um IP aleatório de uma Estação viva no Swarm
	stationURL := os.Getenv("STATION_URL")
	if stationURL == "" {
		stationURL = "http://tasks.station:8081"
	}

	cometURL := os.Getenv("COMET_URL")
	if cometURL == "" {
		cometURL = "http://comet_node:26657"
	}

	drone := &Drone{
		ID:          droneID,
		StationURL:  stationURL,
		CometURL:    cometURL,
		HTTPClient:  server.NewHTTPClient(2 * time.Second), // Timeout curto para o Fail-Safe reagir rápido
	}

	log.Printf("🚁 [%s] Sistemas Online. A iniciar protocolo de patrulha autónoma.", drone.ID)

	// Loop Principal da Máquina de Estados do Drone
	for {
		if !drone.IsRegistered {
			drone.register()
			time.Sleep(2 * time.Second)
			continue
		}

		mission, found := drone.pullMission()
		if !found {
			time.Sleep(2 * time.Second) // Aguarda por ordens
			continue
		}

		// Inicia a missão e monitoriza a ligação à Estação
		success, flightTime := drone.executeMission(mission)

		if success {
			drone.sendAck(mission.Payload.EventID)
			drone.saveReportToLedger(mission, flightTime)
		} else {
			// Fail-Safe ativado: A Estação caiu ou a rede falhou.
			log.Printf("⚠️ [%s] ABORTAR MISSÃO! Ligação C2 perdida. A reconfigurar...", drone.ID)
			drone.IsRegistered = false // Força a procurar uma nova Estação no próximo ciclo
			time.Sleep(3 * time.Second)
		}
	}
}

func (d *Drone) register() {
	url := fmt.Sprintf("%s/api/drone/register", d.StationURL)
	payload := map[string]string{"drone_id": d.ID}
	
	err := d.HTTPClient.PostJSON(url, payload, nil)
	if err == nil {
		d.IsRegistered = true
		log.Printf("🚁 [%s] Registado com sucesso na Estação C2.", d.ID)
	} else {
		log.Printf("⚠️ [%s] Falha ao contactar Estação C2. A tentar novamente...", d.ID)
	}
}

func (d *Drone) pullMission() (model.Mission, bool) {
	var mission model.Mission
	url := fmt.Sprintf("%s/api/mission/pull?drone_id=%s", d.StationURL, d.ID)
	
	err := d.HTTPClient.GetJSON(url, &mission)
	if err != nil || mission.Payload.EventID == "" {
		return mission, false
	}
	return mission, true
}

// executeMission simula o voo e gere o Heartbeat. Retorna true se concluiu, false se ativou o Fail-Safe.
func (d *Drone) executeMission(mission model.Mission) (bool, time.Duration) {
	eventID := mission.Payload.EventID
	log.Printf("🚁 [%s] Em rota para o Alvo: %s (Setor %d)", d.ID, eventID[:8], mission.Payload.SectorID)

	flightDuration := time.Duration(rand.Intn(6)+4) * time.Second
	
	abortChan := make(chan bool)
	doneChan := make(chan bool)

	// Goroutine do Heartbeat (Protocolo Fail-Safe)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		
		renewURL := fmt.Sprintf("%s/api/mission/renew?event_id=%s", d.StationURL, eventID)
		consecutiveFailures := 0

		for {
			select {
			case <-doneChan:
				return // Voo terminou com sucesso, parar pings
			case <-ticker.C:
				err := d.HTTPClient.GetJSON(renewURL, nil)
				if err != nil {
					consecutiveFailures++
					log.Printf("⚠️ [%s] Ping falhou (%d/3)...", d.ID, consecutiveFailures)
					if consecutiveFailures >= 3 {
						abortChan <- true // Sinaliza a abortagem
						return
					}
				} else {
					consecutiveFailures = 0 // Recuperou a ligação, reseta o contador
				}
			}
		}
	}()

	// Aguarda o desfecho da missão (Sucesso vs Abortagem)
	select {
	case <-abortChan:
		return false, 0 // Estação caiu
	case <-time.After(flightDuration):
		doneChan <- true // Avisa a goroutine para parar
		return true, flightDuration
	}
}

func (d *Drone) sendAck(eventID string) {
	url := fmt.Sprintf("%s/api/mission/ack", d.StationURL)
	payload := map[string]string{
		"event_id": eventID,
		"drone_id": d.ID,
	}
	_ = d.HTTPClient.PostJSON(url, payload, nil)
}

func (d *Drone) saveReportToLedger(mission model.Mission, flightTime time.Duration) {
	reportData := map[string]interface{}{
		"drone_id":    d.ID,
		"target":      mission.Payload.SensorID,
		"sector":      mission.Payload.SectorID,
		"duration_ms": flightTime.Milliseconds(),
		"status":      "NEUTRALIZED",
		"timestamp":   time.Now().Unix(),
	}
	reportBytes, _ := json.Marshal(reportData)

	tx := LedgerTransaction{
		Type:      "SAVE_REPORT",
		NationID:  mission.Payload.NationID,
		EventID:   mission.Payload.EventID,
		Signature: fmt.Sprintf("SIG-%s", d.ID),
		Payload:   string(reportBytes),
	}
	txBytes, _ := json.Marshal(tx)
	
	rpcPayload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "broadcast_tx_sync",
		"params": map[string]string{
			"tx": base64.StdEncoding.EncodeToString(txBytes),
		},
	}

	err := d.HTTPClient.PostJSON(d.CometURL, rpcPayload, nil)
	if err != nil {
		log.Printf("❌ [%s] Erro Crítico: Falha ao auditar missão na Blockchain: %v", d.ID, err)
		return
	}
	log.Printf("✅ [%s] Alvo %s Neutralizado e Auditado no Ledger (Imutável).", d.ID, mission.Payload.EventID[:8])
}