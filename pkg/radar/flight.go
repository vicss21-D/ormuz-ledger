package radar

import (
	"sync"
	"time"

	"ormuz-ledger/pkg/queue"
	"ormuz-ledger/pkg/model"
)

// InFlightRecord guarda a missão e o momento exato em que o contrato (Lease) dela acaba
type InFlightRecord struct {
	Mission   model.Mission
	ExpiresAt time.Time
}

// InFlightManager gerencia as missões que saíram da Heap e estão nas mãos dos Drones
type InFlightManager struct {
	mu      sync.RWMutex
	records map[string]InFlightRecord
	pq      *queue.PriorityQueue // Referência à Heap para podermos devolver missões que falharam
}