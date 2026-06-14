package radar

import (
	"sync"
	"time"

	"strait-of-ormuz/pkg/queue"
	"strait-of-ormuz/pkg/model"
)

// InFlightRecord guarda a missão e o momento exato em que o contrato (Lease) dela acaba
type InFlightRecord struct {
	Mission   models.Mission
	ExpiresAt time.Time
}

// InFlightManager gerencia as missões que saíram da Heap e estão nas mãos dos Drones
type InFlightManager struct {
	mu      sync.RWMutex
	records map[string]InFlightRecord
	pq      *queue.PriorityQueue // Referência à Heap para podermos devolver missões que falharam
}