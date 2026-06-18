package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"ormuz-ledger/internal/ledger"

	abciserver "github.com/cometbft/cometbft/abci/server"
)

// main initializes and runs the blockchain state machine server.
func main() {
	addr := flag.String("addr", "tcp://0.0.0.0:26658", "Endereço do socket ABCI")
	flag.Parse()

	log.Println("[LEDGER] A iniciar o Validador do Estado do Consórcio...")

	// Cria a pasta para o banco de dados persistente
	dbPath := "/app/data/state.db"
	if err := os.MkdirAll("/app/data", os.ModePerm); err != nil {
		log.Fatalf("Erro ao criar pasta do DB: %v", err)
	}

	// Instancia a nossa máquina com o banco embutido
	app := ledger.NewOrmuzLedgerApp(dbPath)
	defer app.Close() // Fecha o banco no encerramento

	// Inicia o servidor ABCI
	server := abciserver.NewSocketServer(*addr, app)
	if err := server.Start(); err != nil {
		log.Fatalf("Erro ao iniciar servidor ABCI: %v", err)
	}
	defer server.Stop()

	log.Printf("[LEDGER] Servidor ABCI ativo em %s. A aguardar motor CometBFT v1.0.1...", *addr)

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Println("[LEDGER] A desligar o validador de estado.")
}
