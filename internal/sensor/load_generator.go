package generator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"time"

	"ormuz-ledger/internal/domain/sensor"
	"github.com/google/uuid"
)

type Engine struct {
	TotalSectors        int
	Frequency           time.Duration
	ThreatProbability   float64
	CriticalProbability float64
	Nations             []string // Lista de Nações do Consórcio
}

// Start inicia o loop gerando dados multiplexados e enviando pelo canal
func (e *Engine) Start(ctx context.Context, out chan<- sensor.Telemetry) {

	ticker := time.NewTicker(e.Frequency)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[GERADOR DE CARGA] Desligando motor multiplexado...")
			return
		case <-ticker.C:
			e.fire(out)
		}
	}
}

func (e *Engine) fire(out chan<- sensor.Telemetry) {
	isThreat := rand.Float64() < e.ThreatProbability
	isCritical := false
	eventID := "N/A"
	signature := ""

	// 1. Multiplexação: Escolhe um setor aleatório para simular neste instante
	sectorID := rand.Intn(e.TotalSectors) + 1
	sensorID := fmt.Sprintf("sensor-sec-%02d", sectorID)

	// 2. Simulação de Evento: Decide aleatoriamente se é um evento de ameaça ou não
	nationIndex := rand.Intn(len(e.Nations))
	nationID := e.Nations[nationIndex]

	if isThreat {
		eventID = uuid.New().String()
		isCritical = rand.Float64() < e.CriticalProbability

		// 3. Simulação de Assinatura Criptográfica
		// O hash atrela a identidade da nação que está pagando pelo evento gerado.
		rawSig := fmt.Sprintf("%s:%s:secret-key", eventID, nationID)
		hash := sha256.Sum256([]byte(rawSig))
		signature = hex.EncodeToString(hash[:])
	}

	payload := sensor.NewTelemetry(eventID, sensorID, sectorID, isThreat, isCritical, nationID, signature)

	// Backpressure
	select {
	case out <- payload:
	default:
		// Se o canal encher (brokers lentos), descartamos silenciosamente - Adicionar log de aviso se necessário
	}
}