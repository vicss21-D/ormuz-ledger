package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	cache "ormuz-ledger/internal/inventory"
	"ormuz-ledger/pkg/queue"
)

// main initializes and runs the broker service with mission processing and HTTP API.
func main() {
	// 1. Configurações de Ambiente
	udpPort := getEnv("UDP_PORT", "9000")
	httpPort := getEnv("HTTP_PORT", "8080")
	cometURL := getEnv("COMET_URL", "http://tasks.cometbft:26657")

	// 2. Inicialização dos Componentes Centrais (Fim do Estado Global)
	missionQueue := queue.New(100)
	shadowBuffer := NewShadowBufferManager()
	idempotency := cache.NewIdempotencyFilter(60 * time.Second)
	unverified := NewUnverifiedBufferManager()

	router := NewSectorRouter(25, "tasks.broker")
	ledgerClient := NewLedgerClient(cometURL)

	// 3. Montagem do Processador de Missões
	processor := &MissionProcessor{
		Queue:        missionQueue,
		ShadowBuffer: shadowBuffer,
		Unverified:   unverified,
		Ledger:       ledgerClient,
		Filter:       idempotency,
		Router:       router,
	}

	// 4. Goroutines Auxiliares
	go router.WatchTopology(func() {
		// Quando a topologia muda, o Roteador chama esta função de resgate
		shadowBuffer.RescueOrphanedMissions(router, missionQueue)
	})

	// 5. Início do Servidor HTTP (C2 e Drones)
	httpServer := NewHTTPServer(missionQueue, shadowBuffer, unverified, ledgerClient, idempotency)
	go func() {
		log.Printf("🌐 Servidor HTTP (C2) escutando na porta %s...", httpPort)
		if err := http.ListenAndServe(":"+httpPort, httpServer.SetupRouter()); err != nil {
			log.Fatalf("[FATAL] Servidor HTTP caiu: %v", err)
		}
	}()

	// 6. Início do Consumo UDP (Sensores)
	rawPacketsChan := make(chan []byte, 5000)
	processor.StartWorkers(10, rawPacketsChan)

	startUDPIngestion(udpPort, rawPacketsChan)
}

// startUDPIngestion listens for incoming UDP packets and sends them to the processor.
func startUDPIngestion(port string, outChan chan<- []byte) {
	addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("0.0.0.0:%s", port))
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("[FATAL] Falha no UDP: %v", err)
	}
	defer conn.Close()

	log.Printf("Broker online na porta UDP: %s", port)
	buffer := make([]byte, 1024)

	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}

		packetCopy := make([]byte, n)
		copy(packetCopy, buffer[:n])

		select {
		case outChan <- packetCopy:
		default:
			// Backpressure
		}
	}
}

// getEnv retrieves an environment variable or returns a fallback value.
func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
