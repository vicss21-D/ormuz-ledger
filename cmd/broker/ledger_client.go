package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ormuz-ledger/internal/domain/ledger"
	"ormuz-ledger/pkg/model"
	"ormuz-ledger/pkg/server"
)

// ABCIQueryResponse wraps the ABCI query response from CometBFT.
type ABCIQueryResponse struct {
	Result struct {
		Response struct {
			Code  uint32 `json:"code"`
			Value []byte `json:"value"`
			Log   string `json:"log"`
		} `json:"response"`
	} `json:"result"`
}

// CometRPCResponse wraps the RPC response from CometBFT.
type CometRPCResponse struct {
	Result struct {
		Code int    `json:"code"`
		Log  string `json:"log"`
	} `json:"result"`
}

// LedgerClient communicates with the blockchain ledger via CometBFT RPC.
type LedgerClient struct {
	CometBFTUrl string
	HTTPClient  *server.HTTPClient
}

// NewLedgerClient creates a ledger client connected to CometBFT.
func NewLedgerClient(cometURL string) *LedgerClient {
	return &LedgerClient{
		CometBFTUrl: cometURL,
		HTTPClient:  server.NewHTTPClient(2 * time.Second),
	}
}

func SignTransaction(nationID, txType, eventID string) string {
	// Cria uma seed estrita de 32 bytes baseada no ID da nação
	seedString := fmt.Sprintf("%-32s", nationID+"-SECRET-SEED-ORMUZ-2026")
	seed := []byte(seedString)[:32]

	// Gera a Chave Privada
	privKey := ed25519.NewKeyFromSeed(seed)

	// Assina a mensagem (Tipo + Nação + Evento)
	message := []byte(txType + nationID + eventID)
	signatureBytes := ed25519.Sign(privKey, message)

	// Retorna em Hexadecimal para a struct
	return hex.EncodeToString(signatureBytes)
}

// SpendCredit debits one credit from a nation's balance.
func (lc *LedgerClient) SpendCredit(data model.SensorData) bool {
	tx := ledger.Transaction{
		Type:     "SPEND_CREDIT",
		NationID: data.NationID,
		EventID:  data.EventID,
		// O Broker substitui a assinatura que veio da ponta pela assinatura real da Nação
		Signature: SignTransaction(data.NationID, "SPEND_CREDIT", data.EventID),
	}

	txBytes, _ := json.Marshal(tx)

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "broadcast_tx_sync",
		"params": map[string]string{
			"tx": base64.StdEncoding.EncodeToString(txBytes),
		},
	}

	var rpcResp CometRPCResponse
	err := lc.HTTPClient.PostJSON(lc.CometBFTUrl, payload, &rpcResp)
	if err != nil {
		log.Printf("[BROKER-RPC] Falha de comunicação com o Ledger: %v", err)
		return false
	}

	if rpcResp.Result.Code != 0 {
		log.Printf("[BROKER-RPC] Crédito REJEITADO (Nação: %s). Motivo: %s", data.NationID, rpcResp.Result.Log)
		return false
	}

	return true
}

// SaveReport archives a completed mission report to the immutable ledger.
func (lc *LedgerClient) SaveReport(data model.SensorData, reportPayload []byte) bool {
	tx := ledger.Transaction{
		Type:     "SAVE_REPORT",
		NationID: data.NationID,
		EventID:  data.EventID,
		// O Broker assina a transação documental
		Signature: SignTransaction(data.NationID, "SAVE_REPORT", data.EventID),
		Payload:   string(reportPayload),
	}

	txBytes, _ := json.Marshal(tx)

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "broadcast_tx_sync",
		"params": map[string]string{
			"tx": base64.StdEncoding.EncodeToString(txBytes),
		},
	}

	var rpcResp CometRPCResponse
	err := lc.HTTPClient.PostJSON(lc.CometBFTUrl, payload, &rpcResp)
	if err != nil {
		log.Printf("[BROKER-RPC] Falha de comunicação ao salvar relatório: %v", err)
		return false
	}

	if rpcResp.Result.Code != 0 {
		log.Printf("[BROKER-RPC] Falha ao arquivar relatório (Evento: %s). Motivo: %s", data.EventID, rpcResp.Result.Log)
		return false
	}

	return true
}

// QueryGlobalState retrieves the complete state from the ledger.
func (lc *LedgerClient) QueryGlobalState() ([]byte, error) {
	// 1. Constrói a URL usando o campo correto da sua struct
	url := fmt.Sprintf("%s/abci_query?path=\"state\"", lc.CometBFTUrl)

	var abciResp ABCIQueryResponse

	// 2. Utiliza o método GetJSON do seu pacote pkg/server que já trata Retries e Timeouts
	err := lc.HTTPClient.GetJSON(url, &abciResp)
	if err != nil {
		return nil, fmt.Errorf("falha ao consultar o Ledger: %w", err)
	}

	// 3. Verifica se a aplicação ABCI devolveu algum erro de negócio (ex: Rota não encontrada)
	if abciResp.Result.Response.Code != 0 {
		return nil, fmt.Errorf("erro interno no ledger: %s", abciResp.Result.Response.Log)
	}

	return abciResp.Result.Response.Value, nil
}
