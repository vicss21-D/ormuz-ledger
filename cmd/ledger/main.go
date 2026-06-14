package main

import (
	"flag"
	//"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"ormuz-ledger/internal/ledger"

	abciserver "github.com/cometbft/cometbft/abci/server"
)

func main() {
	// porta 26658
	addr := flag.String("addr", "tcp://0.0.0.0:26658", "Endereço do socket ABCI")
	flag.Parse()

	log.Println("[LEDGER] A iniciar o Validador do Estado do Consórcio...")

	// Instancia a nossa máquina de regras de negócio
	app := ledger.NewOrmuzLedgerApp()

	// Inicia o servidor ABCI que o contentor do CometBFT vai consumir
	server := abciserver.NewSocketServer(*addr, app)
	if err := server.Start(); err != nil {
		log.Fatalf("Erro ao iniciar servidor ABCI: %v", err)
	}
	defer server.Stop()

	log.Printf("[LEDGER] Servidor ABCI ativo em %s. A aguardar conexão do motor CometBFT...", *addr)

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Println("[LEDGER] A desligar o validador de estado.")
}