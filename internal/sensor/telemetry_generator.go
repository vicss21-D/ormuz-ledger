package generator

import (
	"context"
	"log"
	"math/rand"
	"time"

	"ormuz-ledger/internal/domain"
	"github.com/google/uuid"
)

type Engine struct {
	SensorID            string
	SectorID            int
	Frequency           time.Duration
	ThreatProbability   float64
	CriticalProbability float64
}

// Loop gerando dados e enviando pelo canal
func (e *Engine) Start(ctx context.Context, out chan<- domain.Telemetry) {
	
	ticker := time.NewTicker(e.Frequency)
	defer ticker.Stop()

	//rand.Seed(time.Now().UnixNano()) - obsoleto para versões 1.20+

	for {
		select {
		case <-ctx.Done():
			log.Printf("[GERADOR %s] Desligando...", e.SensorID)
			return
		case <-ticker.C:
			e.fire(out)
		}
	}
}

func (e *Engine) fire(out chan<- domain.Telemetry) {
	isThreat := rand.Float64() < e.ThreatProbability
	isCritical := false
	eventID := "N/A"

	if isThreat {
		eventID = uuid.New().String()
		isCritical = rand.Float64() < e.CriticalProbability
	}

	payload := domain.NewTelemetry(eventID, e.SensorID, e.SectorID, isThreat, isCritical)

	// Backpressure
	select {
	case out <- payload:
	default:
		//log.Printf("[GERADOR %s] Aviso: Canal cheio, descartando pacote", e.SensorID)
	}
}