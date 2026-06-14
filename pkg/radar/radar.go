package radar

import (
	"log"
	"time"

	"strait-of-ormuz/pkg/model"
	"strait-of-ormuz/pkg/queue"
)

// NewInFlightManager cria o gerenciador e já inicia o Cão de Guarda (Janitor) em background
func NewInFlightManager(pq *queue.PriorityQueue) *InFlightManager {
	manager := &InFlightManager{
		records: make(map[string]InFlightRecord),
		pq:      pq,
	}

	// Inicia a rotina de limpeza autônoma
	go manager.startJanitor()

	return manager
}

// MarkInFlight registra que uma missão saiu da base e está em execução
func (m *InFlightManager) MarkInFlight(mission models.Mission, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.records[mission.Payload.EventID] = InFlightRecord{
		Mission:   mission,
		ExpiresAt: time.Now().Add(duration),
	}
}

// RenewLease estende o tempo de vida de uma missão (O Drone chama isso enquanto voa)
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

// Acknowledge remove a missão permanentemente (O Drone destruiu o alvo)
func (m *InFlightManager) Acknowledge(eventID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.records[eventID]; !exists {
		return false
	}

	delete(m.records, eventID) // Apaga da memória RAM
	return true
}

// Nack aborta a missão explicitamente (O Drone avisou que falhou)
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

// startJanitor roda a cada 2 segundos varrendo a memória atrás de contratos expirados
func (m *InFlightManager) startJanitor() {
	ticker := time.NewTicker(2 * time.Second)
	for range ticker.C {
		m.sweep()
	}
}

// sweep faz a varredura e pune os drones que perderam a conexão
func (m *InFlightManager) sweep() {
	m.mu.Lock()
	var expired []models.Mission
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