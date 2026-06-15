package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"time"

	"ormuz-ledger/pkg/model"
	"ormuz-ledger/pkg/server" // Sua biblioteca HTTP resiliente
)

type LedgerTransaction struct {
	Type      string `json:"type"`
	NationID  string `json:"nation_id"`
	EventID   string `json:"event_id"`
	Signature string `json:"signature"`
	Payload   string `json:"payload"`
}

type CometRPCResponse struct {
	Result struct {
		Code int    `json:"code"`
		Log  string `json:"log"`
	} `json:"result"`
}

type LedgerClient struct {
	CometBFTUrl string
	HTTPClient  *server.HTTPClient
}

func NewLedgerClient(cometURL string) *LedgerClient {
	return &LedgerClient{
		CometBFTUrl: cometURL,
		HTTPClient:  server.NewHTTPClient(2 * time.Second),
	}
}

func (lc *LedgerClient) SpendCredit(data model.SensorData) bool {
	tx := LedgerTransaction{
		Type:      "SPEND_CREDIT",
		NationID:  data.NationID,
		EventID:   data.EventID,
		Signature: data.Signature,
	}

	txBytes, _ := json.Marshal(tx)

	// Utilizando o padrão JSON-RPC via POST. À prova de corrupção de caracteres.
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "broadcast_tx_sync",
		"params": map[string]string{
			"tx": base64.StdEncoding.EncodeToString(txBytes),
		},
	}

	var rpcResp CometRPCResponse
	// Usa a sua função PostJSON com retry exponencial nativo
	err := lc.HTTPClient.PostJSON(lc.CometBFTUrl, payload, &rpcResp)
	if err != nil {
		log.Printf("[BROKER-RPC] Falha de comunicação com o Ledger: %v", err)
		return false
	}

	// Code 0 significa sucesso no CheckTx do nosso ledger (Tem saldo!)
	if rpcResp.Result.Code != 0 {
		log.Printf("[BROKER-RPC] Crédito REJEITADO (Nação: %s). Motivo: %s", data.NationID, rpcResp.Result.Log)
		return false
	}

	return true
}