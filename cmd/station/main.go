package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"ormuz-ledger/pkg/queue"
	"ormuz-ledger/pkg/radar"
	"ormuz-ledger/pkg/server"
)

// getEnv retrieves an environment variable or returns a fallback value.
func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

// main initializes and runs the station command and control server.
func main() {
	port := getEnv("PORT", "8081")
	brokerURL := getEnv("BROKER_URL", "http://tasks.broker:8080") // Resolve via DNS Swarm

	// 1. Radar Local com Fila Dummy
	// A Estação não gere as filas reais do sistema, serve apenas para intercetar as falhas do Radar
	dummyQueue := queue.New(10)
	flightRadar := radar.NewInFlightManager(dummyQueue)

	httpClient := server.NewHTTPClient(2 * time.Second)

	// 2. Loop de Fallback (Vigia os Drones Abatidos)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		for range ticker.C {
			// Se o Radar colocar algo aqui, significa que o Lease do Drone expirou!
			mission, found := dummyQueue.Dequeue()
			if found {
				log.Printf("⚠️ [C2-JANITOR] Conexão perdida com o Drone (Timeout)! Devolvendo Missão %s ao Broker...", mission.Payload.EventID[:8])

				resolveURL := brokerURL + "/queue/resolve"
				payload := map[string]interface{}{
					"action":  "REQUEUE",
					"mission": mission,
				}
				// Avisa o Broker para devolver a missão à Heap Principal
				_ = httpClient.PostJSON(resolveURL, payload, nil)
			}
		}
	}()

	// 3. Inicia o Servidor de Comando
	stationServer := NewStationServer(brokerURL, flightRadar)

	log.Printf("📡 Estação de Comando (C2) online na porta %s. A aguardar Drones...", port)
	if err := http.ListenAndServe(":"+port, stationServer.SetupRouter()); err != nil {
		log.Fatalf("[FATAL] Falha na Estação: %v", err)
	}
}
