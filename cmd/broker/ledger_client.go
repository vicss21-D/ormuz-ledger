package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ormuz-ledger/pkg/model"
)

// LedgerTransaction reflete a estrutura esperada pela nossa App Go (OrmuzLedgerApp)
type LedgerTransaction struct {
	Type      string `json:"type"`
	NationID  string `json:"nation_id"`
	EventID   string `json:"event_id"`
	Signature string `json:"signature"`
	Payload   string `json:"payload"`
}

// CometRPCResponse mapeia a resposta padrão do CometBFT
type CometRPCResponse struct {
	Result struct {
		Code int    `json:"code"`
		Log  string `json:"log"`
	} `json:"result"`
}

// LedgerClient encapsula a comunicação do Broker com o Nó Blockchain
type LedgerClient struct {
	CometBFTUrl string
	HTTPClient  *http.Client
}

func NewLedgerClient(cometURL string) *LedgerClient {
	return &LedgerClient{
		CometBFTUrl: cometURL,
		HTTPClient: &http.Client{
			Timeout: 2 * time.Second, // Fail-fast: não queremos travar o broker
		},
	}
}

// SpendCredit solicita ao Ledger que debite 1 crédito da Nação
// Retorna 'true' se a blockchain aprovar a transação (Code == 0)
func (lc *LedgerClient) SpendCredit(data model.SensorData) bool {
	// 1. Monta a Transação de Negócio
	tx := LedgerTransaction{
		Type:      "SPEND_CREDIT",
		NationID:  data.NationID,
		EventID:   data.EventID,
		Signature: data.Signature,
	}

	txBytes, err := json.Marshal(tx)
	if err != nil {
		return false
	}

	// 2. O CometBFT exige que a transação venha encodada em Base64 ou Hex na URL
	txBase64 := base64.StdEncoding.EncodeToString(txBytes)

	// Utilizamos o endpoint /broadcast_tx_sync para aguardar o CheckTx (Fase de Mempool)
	// Isso garante que sabemos se a nação tem saldo IMEDIATAMENTE, antes mesmo do bloco fechar.
	url := fmt.Sprintf("%s/broadcast_tx_sync?tx=\"%s\"", lc.CometBFTUrl, txBase64)

	resp, err := lc.HTTPClient.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	defer resp.Body.Close()

	var rpcResp CometRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return false
	}

	// Code 0 significa que a transação passou pelo CheckTx do nosso ledger (Tem saldo!)
	if rpcResp.Result.Code != 0 {
		fmt.Printf("[BROKER-RPC] Crédito REJEITADO para Nação %s (Evento %s). Motivo: %s\n", 
			data.NationID, data.EventID[:8], rpcResp.Result.Log)
		return false
	}

	return true
}