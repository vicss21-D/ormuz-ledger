package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"time"
	"fmt"

	"ormuz-ledger/pkg/model"
	"ormuz-ledger/pkg/server"
	"ormuz-ledger/internal/domain/ledger"
)

type ABCIQueryResponse struct {
	Result struct {
		Response struct {
			Code  uint32 `json:"code"`
			Value []byte `json:"value"`
			Log   string `json:"log"`
		} `json:"response"`
	} `json:"result"`
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
	tx := ledger.Transaction{
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

// SaveReport envia os dados da missão concluída para arquivamento imutável no LevelDB
func (lc *LedgerClient) SaveReport(data model.SensorData, reportPayload []byte) bool {
	tx := ledger.Transaction{
		Type:      "SAVE_REPORT",
		NationID:  data.NationID,
		EventID:   data.EventID,
		Signature: data.Signature,
		Payload:   string(reportPayload),
	}

	txBytes, _ := json.Marshal(tx)

	// Utilizando o mesmo padrão JSON-RPC robusto
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