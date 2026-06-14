package ledger

import (
	"context"
	"encoding/json"
	"log"

	"github.com/cometbft/cometbft/abci/types"
	"ormuz-ledger/internal/domain/ledger" 
)

// OrmuzLedgerApp é a nossa máquina de estados da blockchain
type OrmuzLedgerApp struct {
	types.BaseApplication

	// Estado em Memória
	Balances map[string]int
	Reports  map[string]string // EventID -> Relatório JSON
}

// NewOrmuzLedgerApp inicializa a simulação da economia
func NewOrmuzLedgerApp() *OrmuzLedgerApp {
	return &OrmuzLedgerApp{
		// Distribuição inicial de créditos para o consórcio
		Balances: map[string]int{
			"BR": 1000,
			"UK": 1000,
			"FR": 1000,
			"US": 1000,
		},
		Reports: make(map[string]string),
	}
}

// CheckTx (Fase de Mempool): Chamado pelo CometBFT ANTES do consenso.
func (app *OrmuzLedgerApp) CheckTx(ctx context.Context, req *types.CheckTxRequest) (*types.CheckTxResponse, error) {
	var tx ledger.Transaction
	if err := json.Unmarshal(req.Tx, &tx); err != nil {
		return &types.CheckTxResponse{Code: 1, Log: "JSON malformado"}, nil
	}

	if tx.Type == "SPEND_CREDIT" {
		balance, exists := app.Balances[tx.NationID]
		if !exists || balance <= 0 {
			// Rejeita a transação: não gasta banda da rede nem entra no bloco
			return &types.CheckTxResponse{Code: 2, Log: "Saldo insuficiente ou nação não reconhecida"}, nil
		}
	}

	// Aceita a transação para ir a votação no PBFT
	return &types.CheckTxResponse{Code: 0}, nil
}

// FinalizeBlock (Fase de Bloco): É aqui que a mutação de estado se torna definitiva e imutável.
func (app *OrmuzLedgerApp) FinalizeBlock(ctx context.Context, req *types.FinalizeBlockRequest) (*types.FinalizeBlockResponse, error) {
	// Preparamos um array para guardar o resultado de cada transação
	txResults := make([]*types.ExecTxResult, 0, len(req.Txs))

	for _, txBytes := range req.Txs {
		var tx ledger.Transaction
		
		if err := json.Unmarshal(txBytes, &tx); err != nil {
			txResults = append(txResults, &types.ExecTxResult{Code: 1, Log: "Falha no parse"})
			continue
		}

		switch tx.Type {
		case "SPEND_CREDIT":
			app.Balances[tx.NationID] -= 1
			log.Printf("[LEDGER] Crédito debitado de %s. Saldo atual: %d (Evento: %s)", 
				tx.NationID, app.Balances[tx.NationID], tx.EventID)

		case "SAVE_REPORT":
			app.Reports[tx.EventID] = tx.Payload
			log.Printf("[LEDGER] Relatório arquivado imutavelmente para o Evento: %s", tx.EventID)
		}

		// Marca a transação como Sucesso (Code 0)
		txResults = append(txResults, &types.ExecTxResult{Code: 0})
	}

	// O CometBFT usará essa resposta para construir o Hash do Bloco
	return &types.FinalizeBlockResponse{
		TxResults: txResults,
	}, nil
}

// Commit é chamado quando o bloco é fechado. 
func (app *OrmuzLedgerApp) Commit(ctx context.Context, req *types.CommitRequest) (*types.CommitResponse, error) {
	return &types.CommitResponse{}, nil
}