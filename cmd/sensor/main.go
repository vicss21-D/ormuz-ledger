package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"ormuz-ledger/internal/domain"
	"github.com/joho/godotenv"

	generator "ormuz-ledger/internal/sensor"
)

const (
	BrokerDNS  = "tasks.broker"
	BrokerPort = "9000"
)

var (
	brokerList    []string
	brokersMutex  sync.RWMutex
	previousState string
)

func watchBrokerDNS(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ips, err := net.LookupIP(BrokerDNS)
			if err != nil || len(ips) == 0 {
				continue
			}

			var ipStrs []string
			for _, ip := range ips {
				ipStrs = append(ipStrs, ip.String())
			}
			sort.Strings(ipStrs)

			currentState := strings.Join(ipStrs, ",")
			if currentState != previousState {
				brokersMutex.Lock()
				brokerList = ipStrs
				brokersMutex.Unlock()

				previousState = currentState
				log.Printf("[DNS] Brokers descobertos: %d | %v", len(ipStrs), ipStrs)
			}
		}
	}
}

func sendToBroker(brokerIP string, data []byte) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%s", brokerIP, BrokerPort))
	if err != nil {
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.Write(data)
}

func main() {
	// 1. Carregamento do Arquivo .env
	// Se o arquivo não existir (ex: rodando direto no cluster Swarm de produção), 
	// ele ignora o erro e continua usando as variáveis de ambiente injetadas pelo SO/Docker.
	if err := godotenv.Load(); err != nil {
		log.Println("[BOOT] Arquivo .env não encontrado. Utilizando variáveis do ambiente hospedeiro.")
	}

	// 2. Configurações Globais da Simulação via Variáveis de Ambiente
	totalSectors, _ := strconv.Atoi(getEnv("TOTAL_SECTORS", "25"))
	freqMs, _ := strconv.Atoi(getEnv("FREQ_MS", "500"))
	threatProb, _ := strconv.ParseFloat(getEnv("THREAT_PROB", "0.05"), 64)
	critProb, _ := strconv.ParseFloat(getEnv("CRITICAL_PROB", "0.20"), 64)

	// Captura a lista de nações dinamicamente
	nationsStr := getEnv("NATIONS", "BR,UK,FR,US")
	nations := strings.Split(nationsStr, ",")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3. Monitoramento de Brokers
	time.Sleep(1 * time.Second)
	go watchBrokerDNS(ctx)

	log.Printf("[BOOT] Gerador de Carga Multiplexado Iniciado.")
	log.Printf("[BOOT] Parâmetros: %d Setores | Freq: %dms | Nações: %v", totalSectors, freqMs, nations)

	// 4. Inicialização do Motor
	telemetryChan := make(chan domain.Telemetry, 500)

	engine := &generator.Engine{
		TotalSectors:        totalSectors,
		Frequency:           time.Duration(freqMs) * time.Millisecond,
		ThreatProbability:   threatProb,
		CriticalProbability: critProb,
		Nations:             nations,
	}
	go engine.Start(ctx, telemetryChan)

	// 5. Ingress (Multicast para os Brokers)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case payload := <-telemetryChan:
				data, err := json.Marshal(payload)
				if err != nil {
					continue
				}

				brokersMutex.RLock()
				brokerIPs := make([]string, len(brokerList))
				copy(brokerIPs, brokerList)
				brokersMutex.RUnlock()

				for _, brokerIP := range brokerIPs {
					go sendToBroker(brokerIP, data)
				}
			}
		}
	}()

	// 6. Graceful Shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Printf("[SHUTDOWN] Encerrando simulador.")
	cancel()
	time.Sleep(500 * time.Millisecond)
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}