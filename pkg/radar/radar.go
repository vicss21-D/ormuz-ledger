package radar

import (
	"log"
	"time"

	"ormuz-ledger/pkg/model"
	"ormuz-ledger/pkg/queue"
)

// NewInFlightManager creates an in-flight manager and starts the janitor cleanup goroutine.
func NewInFlightManager(pq *queue.PriorityQueue) *InFlightManager {
	manager := &InFlightManager{
		records: make(map[string]InFlightRecord),
		pq:      pq,
	}

	// Inicia a rotina de limpeza autônoma
	go manager.startJanitor()

	return manager
}

// MarkInFlight records that a mission is now in-flight with a specified duration.
func (m *InFlightManager) MarkInFlight(mission model.Mission, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.records[mission.Payload.EventID] = InFlightRecord{
		Mission:   mission,
		ExpiresAt: time.Now().Add(duration),
	}
}

// RenewLease extends the mission lease expiration time.
func (m *InFlightManager) RenewLease(eventID string, duration time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, exists := m.records[eventID]
	if !exists {
		return false // Missão já foi concluída, abortada ou o Janitor já a recolheu
	}

	// Se por acaso o pedido de renovação chegou um milissegundo DEPOIS de expirar, rejeitamos
	if time.Now().After(record.ExpiresAt) {
		return false
	}

	// Estende o contrato
	record.ExpiresAt = time.Now().Add(duration)
	m.records[eventID] = record
	return true
}

// Acknowledge marks a mission as completed and removes it from tracking.
func (m *InFlightManager) Acknowledge(eventID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.records[eventID]; !exists {
		return false
	}

	delete(m.records, eventID) // Apaga da memória RAM
	return true
}

// Nack aborts a mission and re-queues it for retry.
func (m *InFlightManager) Nack(eventID string) bool {
	m.mu.Lock()
	record, exists := m.records[eventID]
	if exists {
		delete(m.records, eventID)
	}
	m.mu.Unlock()

	// Se a missão existia, nós a devolvemos para a Fila de Prioridade (Heap)
	if exists {
		m.pq.Enqueue(record.Mission) // *Nota: Ajuste para o nome da função que insere na sua Heap (ex: Push, Insert)
		return true
	}
	return false
}

// =========================================================
// Watchdog
// =========================================================

// startJanitor runs periodically to sweep expired lease records.
func (m *InFlightManager) startJanitor() {
	ticker := time.NewTicker(2 * time.Second)
	for range ticker.C {
		m.sweep()
	}
}

// sweep collects expired missions and re-queues them to the priority queue.
func (m *InFlightManager) sweep() {
	m.mu.Lock()
	var expired []model.Mission
	now := time.Now()

	for id, record := range m.records {
		if now.After(record.ExpiresAt) {
			// O contrato venceu! Separa a missão para ser resgatada e apaga do rastreio
			expired = append(expired, record.Mission)
			delete(m.records, id)
		}
	}
	m.mu.Unlock()

	// Devolve as missões expiradas para a Heap
	for _, miss := range expired {
		log.Printf("JANITOR: Drone perdeu conexão! Missão %s expirou por falta de Lease. Devolvendo à Heap...", miss.Payload.EventID[:8])
		m.pq.Enqueue(miss)
	}
}
