package main

import (
	"encoding/base64"
	"encoding/json"
	"encoding/hex"
	"crypto/ed25519"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"ormuz-ledger/pkg/model"
	"ormuz-ledger/pkg/server"
)

// LedgerTransaction represents a transaction to be recorded on the blockchain.
type LedgerTransaction struct {
	Type      string `json:"type"`
	NationID  string `json:"nation_id"`
	EventID   string `json:"event_id"`
	Signature string `json:"signature"`
	Payload   string `json:"payload"`
}

// Drone is an autonomous unmanned aerial vehicle.
type Drone struct {
	ID               string
	DiscoveryURL     string // O DNS nativo do Swarm (tasks.station)
	ActiveStationURL string // O IP fixo da Estação que o adotou
	CometURL         string
	HTTPClient       *server.HTTPClient
	IsRegistered     bool
}

// main initializes and runs the autonomous drone agent.
func main() {
	droneID := os.Getenv("DRONE_ID")
	if droneID == "" {
		droneID = fmt.Sprintf("UAV-%d", rand.Intn(9000)+1000)
	}

	stationURL := os.Getenv("STATION_URL")
	if stationURL == "" {
		stationURL = "http://tasks.station:8081"
	}

	cometURL := os.Getenv("COMET_URL")
	if cometURL == "" {
		cometURL = "http://comet_node:26657"
	}

	drone := &Drone{
		ID:           droneID,
		DiscoveryURL: stationURL,
		CometURL:     cometURL,
		HTTPClient:   server.NewHTTPClient(2 * time.Second),
	}

	log.Printf("🚁 [%s] Sistemas Online. A iniciar protocolo autónomo.", drone.ID)

	for {
		if !drone.IsRegistered {
			drone.register()
			time.Sleep(2 * time.Second)
			continue
		}

		mission, found := drone.pullMission()
		if !found {
			time.Sleep(2 * time.Second)
			continue
		}

		success, flightTime := drone.executeMission(mission)

		if success {
			drone.sendAck(mission.Payload.EventID)
			drone.saveReportToLedger(mission, flightTime)
		} else {
			log.Printf("⚠️ [%s] ABORTAR MISSÃO! Ligação C2 perdida. A reconfigurar...", drone.ID)
			drone.IsRegistered = false
			drone.ActiveStationURL = "" // Limpa a sessão morta
			time.Sleep(3 * time.Second)
		}
	}
}

// register connects the drone to the station.
func (d *Drone) register() {
	url := fmt.Sprintf("%s/api/drone/register", d.DiscoveryURL)
	payload := map[string]string{"drone_id": d.ID}

	var response struct {
		DirectURL string `json:"direct_url"`
	}

	err := d.HTTPClient.PostJSON(url, payload, &response)
	if err == nil && response.DirectURL != "" {
		d.ActiveStationURL = response.DirectURL
		d.IsRegistered = true
		log.Printf("🚁 [%s] Ligação C2 fixada com a Estação: %s", d.ID, d.ActiveStationURL)
	} else {
		log.Printf("⚠️ [%s] Falha ao registar. A tentar novamente...", d.ID)
	}
}

// pullMission retrieves the next mission from the station.
func (d *Drone) pullMission() (model.Mission, bool) {
	var mission model.Mission
	url := fmt.Sprintf("%s/api/mission/pull?drone_id=%s", d.ActiveStationURL, d.ID)

	err := d.HTTPClient.GetJSON(url, &mission)
	if err != nil || mission.Payload.EventID == "" {
		return mission, false
	}
	return mission, true
}

// executeMission flies to target and maintains heartbeat lease renewal.
func (d *Drone) executeMission(mission model.Mission) (bool, time.Duration) {
	eventID := mission.Payload.EventID
	log.Printf("🚁 [%s] Em rota para o Alvo: %s (Setor %d)", d.ID, eventID[:8], mission.Payload.SectorID)

	flightDuration := time.Duration(rand.Intn(6)+4) * time.Second
	abortChan := make(chan bool)
	doneChan := make(chan bool)

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		renewURL := fmt.Sprintf("%s/api/mission/renew?event_id=%s", d.ActiveStationURL, eventID)
		consecutiveFailures := 0

		for {
			select {
			case <-doneChan:
				return
			case <-ticker.C:
				err := d.HTTPClient.GetJSON(renewURL, nil)
				if err != nil {
					consecutiveFailures++
					log.Printf("⚠️ [%s] Ping falhou (%d/3)...", d.ID, consecutiveFailures)
					if consecutiveFailures >= 3 {
						abortChan <- true
						return
					}
				} else {
					consecutiveFailures = 0
				}
			}
		}
	}()

	select {
	case <-abortChan:
		return false, 0
	case <-time.After(flightDuration):
		doneChan <- true
		return true, flightDuration
	}
}

// sendAck confirms mission completion to the station.
func (d *Drone) sendAck(eventID string) {
	url := fmt.Sprintf("%s/api/mission/ack", d.ActiveStationURL)
	payload := map[string]string{"event_id": eventID, "drone_id": d.ID}
	_ = d.HTTPClient.PostJSON(url, payload, nil)
}

// saveReportToLedger records mission completion on the blockchain.
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
		Signature: SignTransaction(mission.Payload.NationID, "SAVE_REPORT", mission.Payload.EventID),
		Payload:   string(reportBytes),
	}
	txBytes, _ := json.Marshal(tx)

	rpcPayload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "broadcast_tx_sync",
		"params":  map[string]string{"tx": base64.StdEncoding.EncodeToString(txBytes)},
	}

	err := d.HTTPClient.PostJSON(d.CometURL, rpcPayload, nil)
	if err != nil {
		log.Printf("❌ [%s] Falha ao auditar missão na Blockchain: %v", d.ID, err)
		return
	}
	log.Printf("✅ [%s] Alvo %s Neutralizado e Auditado no Ledger.", d.ID, mission.Payload.EventID[:8])
}

func SignTransaction(nationID, txType, eventID string) string {
	seedString := fmt.Sprintf("%-32s", nationID+"-SECRET-SEED-ORMUZ-2026")
	seed := []byte(seedString)[:32]
	privKey := ed25519.NewKeyFromSeed(seed)
	message := []byte(txType + nationID + eventID)
	signatureBytes := ed25519.Sign(privKey, message)
	return hex.EncodeToString(signatureBytes)
}