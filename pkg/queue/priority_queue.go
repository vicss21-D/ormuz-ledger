package queue

import (
	"container/heap"
	"log"
	"sync"
	"time"

	"ormuz-ledger/pkg/clock"
	"ormuz-ledger/pkg/model"
)

// --- INTERNAL HEAP STRUCTURE ---

// missionHeap implements a min-max heap ordering missions by criticality and Lamport clock.
type missionHeap []model.Mission

// Len returns the number of missions in the heap.
func (h missionHeap) Len() int { return len(h) }

// Swap exchanges two missions in the heap.
func (h missionHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Less defines the total ordering: critical first, then by Lamport clock, then by sensor ID.
func (h missionHeap) Less(i, j int) bool {
	// 1. Criticidade Absoluta (Max-Heap: true sobe, false desce)
	if h[i].Payload.IsCritical != h[j].Payload.IsCritical {
		return h[i].Payload.IsCritical
	}

	// 2. Causalidade de Lamport (Min-Heap temporal: menor tempo sai primeiro)
	if h[i].LamportClock != h[j].LamportClock {
		return h[i].LamportClock < h[j].LamportClock
	}

	// 3. Tie-Breaker determinístico (Desempate por ID)
	return h[i].Payload.SensorID < h[j].Payload.SensorID
}

// Push appends a mission to the heap.
func (h *missionHeap) Push(x interface{}) {
	*h = append(*h, x.(model.Mission))
}

// Pop removes and returns the mission with highest priority from the heap.
func (h *missionHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

// --- PRIORITY QUEUE MANAGER (THREAD-SAFE + AGING) ---

// PriorityQueue manages mission scheduling with priority enforcement and starvation prevention.
type PriorityQueue struct {
	mh         *missionHeap
	mu         sync.Mutex
	cond       *sync.Cond
	agingLimit uint64 // Limite de Lamport Ticks antes de promover um pacote
}

// New creates a priority queue and starts the anti-starvation worker.
func New(agingLimit uint64) *PriorityQueue {
	pq := &PriorityQueue{
		mh:         &missionHeap{},
		agingLimit: agingLimit,
	}
	pq.cond = sync.NewCond(&pq.mu)
	heap.Init(pq.mh)

	go pq.antiStarvationWorker()
	return pq
}

// Enqueue adds a mission to the queue in O(log N) time.
func (pq *PriorityQueue) Enqueue(m model.Mission) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	heap.Push(pq.mh, m)
}

// Dequeue removes and returns the highest priority mission. Returns false if queue is empty.
func (pq *PriorityQueue) Dequeue() (model.Mission, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	// Se estiver vazia, não dorme. Apenas avisa quem chamou que não há nada.
	if pq.mh.Len() == 0 {
		return model.Mission{}, false
	}

	mission := heap.Pop(pq.mh).(model.Mission)
	return mission, true
}

// antiStarvationWorker periodically promotes aged non-critical missions to prevent starvation.
func (pq *PriorityQueue) antiStarvationWorker() {
	for {
		time.Sleep(1 * time.Second) // Frequência da varredura

		pq.mu.Lock()
		if pq.mh.Len() == 0 {
			pq.mu.Unlock()
			continue
		}

		currentClock := clock.Get()
		promoted := false

		// Varre a fila procurando esquecidos
		for i := 0; i < pq.mh.Len(); i++ {
			mission := &(*pq.mh)[i]
			if !mission.Payload.IsCritical {
				// Se envelheceu além do limite, promove e carimba
				if currentClock > mission.LamportClock && (currentClock-mission.LamportClock) > pq.agingLimit {
					mission.Payload.IsCritical = true
					promoted = true
				}
			}
		}

		// Se alguém foi promovido, a árvore precisa ser balanceada O(N)
		if promoted {
			heap.Init(pq.mh)
		}
		pq.mu.Unlock()
	}
}

// PrintAllMissions logs the current queue state to stdout.
func (mq *PriorityQueue) PrintAllMissions() {
	// 1. Congela a fila para impedir que o Radar insira itens enquanto lemos
	mq.mu.Lock()
	defer mq.mu.Unlock()

	// Substitua 'mq.items' pelo nome do slice/array real da sua struct
	total := len(*mq.mh)

	if total == 0 {
		log.Println("🛡️ [HEAP] A fila de missões está vazia. Setor limpo.")
		return
	}

	log.Printf("📊 [HEAP] === RAIO-X DO CAMPO DE BATALHA (%d Alvos Pendentes) ===", total)

	for i, mission := range *mq.mh {
		// Ajuste os campos (EventID, ThreatLevel) conforme o seu modelo Mission
		eventID := mission.Payload.EventID
		if len(eventID) > 8 {
			eventID = eventID[:8]
		}

		log.Printf("  -> [%d] Alvo: %s | Nível de Ameaça: %t | Setor: %s",
			i,
			eventID,
			mission.Payload.IsCritical,
			mission.Payload.SensorID,
		)
	}

	log.Println("==================================================================")
}
