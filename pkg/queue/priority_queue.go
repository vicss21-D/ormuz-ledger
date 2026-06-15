package queue

import (
	"container/heap"
	"sync"
	"time"
	"log"

	"ormuz-ledger/pkg/clock"
	"ormuz-ledger/pkg/model"
)

// --- ESTRUTURA INTERNA DA HEAP ---

type missionHeap []model.Mission

func (h missionHeap) Len() int      { return len(h) }
func (h missionHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Less implementa a Ordenação Total Matemática
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

func (h *missionHeap) Push(x interface{}) {
	*h = append(*h, x.(model.Mission))
}

func (h *missionHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

// --- GERENCIADOR DA FILA (THREAD-SAFE + AGING) ---

type PriorityQueue struct {
	mh         *missionHeap
	mu         sync.Mutex
	cond       *sync.Cond
	agingLimit uint64 // Limite de Lamport Ticks antes de promover um pacote
}

// New cria a fila e liga o motor Anti-Starvation
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

// Enqueue enfileira uma nova missão logaritmicamente O(log N)
func (pq *PriorityQueue) Enqueue(m model.Mission) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	heap.Push(pq.mh, m)
}

// Dequeue tenta retirar a missão de maior prioridade da Heap.
// Retorna a missão e 'true' se houver sucesso.
// Se a fila estiver vazia, retorna 'false' imediatamente (Não-bloqueante).
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

// antiStarvationWorker promove missões antigas (Aging)
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

	// PrintAllMissions exibe no terminal o estado atual do campo de batalha aguardando despacho
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