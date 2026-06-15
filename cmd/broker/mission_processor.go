package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	cache "ormuz-ledger/internal/inventory"
	"ormuz-ledger/pkg/clock"
	"ormuz-ledger/pkg/model"
	"ormuz-ledger/pkg/queue"
)

type MissionProcessor struct {
	Queue        *queue.PriorityQueue
	ShadowBuffer *ShadowBufferManager
	Unverified   *UnverifiedBufferManager
	Ledger       *LedgerClient
	Filter       *cache.IdempotencyFilter
	Router       *SectorRouter
}

func (p *MissionProcessor) process(rawBytes []byte, workerID int) {
	var data model.SensorData
	if err := json.Unmarshal(rawBytes, &data); err != nil {
		return
	}

	if !data.IsRequisition || data.SensorID == "" || data.SectorID <= 0 {
		return
	}

	// BLINDAGEM: Se o pacote cru chegar duas vezes (via UDP e via Gossip do Secundário),
	// o Filtro de Idempotência o destrói aqui na entrada.
	if !p.Filter.CheckAndAdd(data.EventID) {
		return
	}

	currentClock := clock.Tick()
	if data.InternalClock > 0 {
		currentClock = clock.Sync(data.InternalClock)
	}

	mission := model.Mission{
		Payload:      data,
		LamportClock: currentClock,
	}

	ownerIP := p.Router.GetOwnerIP(data.SectorID)

	// SE NÃO SOMOS O DONO (Secundário)
	if ownerIP != "" && !isLocalIP(ownerIP) {
		p.Unverified.Store(mission) // Coloca na Sala de Espera

		// Upstream Gossip: Repassa para garantir que o Primário saiba do evento
		data.InternalClock = currentClock
		rawWithClock, _ := json.Marshal(data)
		forwardPacket(ownerIP, rawWithClock)
		return
	}

	// SE SOMOS O DONO (Primário)
	if p.Ledger.SpendCredit(data) {
		// Aprovado: Enfileira para combate
		p.Queue.Enqueue(mission)
		log.Printf("[WORKER %02d] SETOR-%02d PAGO. Enfileirado.", workerID, data.SectorID)

		// Downstream Gossip: Avisa os irmãos para promoverem para o ShadowBuffer Oficial
		p.broadcastValidationSync("ACK", mission)
	} else {
		log.Printf("[WORKER %02d] Rejeição Ledger para Nação %s.", workerID, data.NationID)

		// Downstream Gossip: Avisa os irmãos para descartarem da Sala de Espera
		p.broadcastValidationSync("NACK", mission)
	}
}

// broadcastValidationSync faz o Gossip reverso aos brokers irmãos
func (p *MissionProcessor) broadcastValidationSync(action string, mission model.Mission) {
	ips := p.Router.GetAllMembers() // Retorna todos os IPs do anel
	myIP := getLocalIP()

	payload := map[string]interface{}{
		"action":  action,
		"mission": mission,
	}
	jsonData, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 2 * time.Second}

	for _, ip := range ips {
		if ip == myIP {
			continue
		}
		url := fmt.Sprintf("http://%s:8080/internal/shadow/sync", ip)
		go func(targetURL string) {
			req, _ := http.NewRequest(http.MethodPost, targetURL, bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")
			_, _ = client.Do(req)
		}(url)
	}
}

// StartWorkers inicia N goroutines para processar pacotes UDP
func (p *MissionProcessor) StartWorkers(numWorkers int, packetsChan <-chan []byte) {
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			log.Printf("[WORKER %02d] Online. Aguardando pacotes...", workerID)
			for rawBytes := range packetsChan {
				p.process(rawBytes, workerID)
			}
			log.Printf("[WORKER %02d] Desligado.", workerID)
		}(i)
	}
}
