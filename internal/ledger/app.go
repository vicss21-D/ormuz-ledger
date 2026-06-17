package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/cometbft/cometbft/abci/types"
	"github.com/syndtr/goleveldb/leveldb"
	"ormuz-ledger/internal/domain/ledger"
)

// OrmuzLedgerApp é a nossa máquina de estados da blockchain
type OrmuzLedgerApp struct {
	types.BaseApplication

	// Estado Persistente (Substitui os Maps em memória)
	db *leveldb.DB
}

// NewOrmuzLedgerApp inicializa a simulação da economia com banco local
func NewOrmuzLedgerApp(dbPath string) *OrmuzLedgerApp {
	db, err := leveldb.OpenFile(dbPath, nil)
	if err != nil {
		log.Fatalf("[FATAL] Falha ao abrir o LevelDB: %v", err)
	}

	// Distribuição inicial de créditos
	nations := []string{"BR", "UK", "FR", "US"}
	for _, n := range nations {
		key := []byte(fmt.Sprintf("balance:%s", n))
		has, _ := db.Has(key, nil)
		if !has {
			db.Put(key, []byte("1000"), nil)
			log.Printf("🏦 Nação %s inicializada com 1000 créditos.", n)
		}
	}

	return &OrmuzLedgerApp{db: db}
}

// Fechar conexão com o banco ao desligar o nó
func (app *OrmuzLedgerApp) Close() {
	app.db.Close()
}

// CheckTx (Fase de Mempool): Chamado pelo CometBFT ANTES do consenso.
func (app *OrmuzLedgerApp) CheckTx(ctx context.Context, req *types.CheckTxRequest) (*types.CheckTxResponse, error) {
	var tx ledger.Transaction
	if err := json.Unmarshal(req.Tx, &tx); err != nil {
		return &types.CheckTxResponse{Code: 1, Log: "JSON malformado"}, nil
	}

	if tx.Type == "SPEND_CREDIT" {
		balanceBytes, err := app.db.Get([]byte(fmt.Sprintf("balance:%s", tx.NationID)), nil)
		if err != nil {
			return &types.CheckTxResponse{Code: 2, Log: "Nação não reconhecida"}, nil
		}
		
		balance, _ := strconv.Atoi(string(balanceBytes))
		if balance <= 0 {
			// Rejeita a transação
			return &types.CheckTxResponse{Code: 3, Log: "Saldo insuficiente"}, nil
		}
	}

	return &types.CheckTxResponse{Code: 0}, nil
}

// FinalizeBlock (Fase de Bloco): É aqui que a mutação de estado se torna definitiva.
func (app *OrmuzLedgerApp) FinalizeBlock(ctx context.Context, req *types.FinalizeBlockRequest) (*types.FinalizeBlockResponse, error) {
	txResults := make([]*types.ExecTxResult, 0, len(req.Txs))

	for _, txBytes := range req.Txs {
		var tx ledger.Transaction
		
		if err := json.Unmarshal(txBytes, &tx); err != nil {
			txResults = append(txResults, &types.ExecTxResult{Code: 1, Log: "Falha no parse"})
			continue
		}

		// 1. PREVENÇÃO DE DUPLICAÇÃO DESACOPLADA
		// A chave agora inclui o Tipo. Ex: processed:SPEND_CREDIT_68124a7c... e processed:SAVE_REPORT_68124a7c...
		processedKey := []byte(fmt.Sprintf("processed:%s_%s", tx.Type, tx.EventID))
		hasBeenProcessed, _ := app.db.Has(processedKey, nil)
		
		if hasBeenProcessed {
			log.Printf("⚠️ Operação Duplicada rejeitada! Tipo: %s | Evento: %s", tx.Type, tx.EventID[:8])
			txResults = append(txResults, &types.ExecTxResult{Code: 4, Log: "Transação já processada"})
			continue
		}

		// 2. MÁQUINA DE ESTADOS (Responsabilidade Única)
		switch tx.Type {
		case "SPEND_CREDIT":
			// Apenas altera saldo, não grava relatório
			balanceKey := []byte(fmt.Sprintf("balance:%s", tx.NationID))
			balanceBytes, _ := app.db.Get(balanceKey, nil)
			balance, _ := strconv.Atoi(string(balanceBytes))
			
			newBalance := balance - 1
			app.db.Put(balanceKey, []byte(strconv.Itoa(newBalance)), nil)
			log.Printf("[LEDGER] Crédito debitado de %s. Saldo: %d (Evento: %s)", tx.NationID, newBalance, tx.EventID[:8])

		case "SAVE_REPORT":
			// Apenas grava relatório, não altera saldo
			// Gravamos txBytes inteiro para o Explorer conseguir parsear a estrutura JSON perfeitamente
			reportKey := []byte(fmt.Sprintf("report:%s", tx.EventID))
			app.db.Put(reportKey, txBytes, nil)
			log.Printf("[LEDGER] Relatório arquivado imutavelmente. (Evento: %s)", tx.EventID[:8])
		}

		// 3. APLICA A TRAVA ESPECÍFICA
		app.db.Put(processedKey, []byte("true"), nil)
		txResults = append(txResults, &types.ExecTxResult{Code: 0})
	}

	return &types.FinalizeBlockResponse{
		TxResults: txResults,
		AppHash:   []byte{},
	}, nil
}

// Consulta em O(1) pelo Broker
func (app *OrmuzLedgerApp) Query(ctx context.Context, req *types.QueryRequest) (*types.QueryResponse, error) {
	// Nova rota de exploração global do Estado
	if req.Path == "state" {
		state := make(map[string]interface{})
		balances := make(map[string]int)
		reports := make(map[string]string)

		// Varre o banco de dados local para recolher o estado atual
		iter := app.db.NewIterator(nil, nil)
		defer iter.Release()
		
		for iter.Next() {
			key := string(iter.Key())
			val := string(iter.Value())
			
			if strings.HasPrefix(key, "balance:") {
				nation := strings.Split(key, ":")[1]
				b, _ := strconv.Atoi(val)
				balances[nation] = b
			} else if strings.HasPrefix(key, "report:") {
				eventID := strings.Split(key, ":")[1]
				reports[eventID] = val
			}
		}

		state["balances"] = balances
		state["reports"] = reports

		stateBytes, _ := json.Marshal(state)
		return &types.QueryResponse{Code: 0, Value: stateBytes}, nil
	}

	// Rota antiga de saldo específico (mantida por compatibilidade)
	parts := strings.Split(req.Path, "/")
	if len(parts) == 2 && parts[0] == "balance" {
		nation := parts[1]
		balanceBytes, err := app.db.Get([]byte(fmt.Sprintf("balance:%s", nation)), nil)
		if err != nil {
			return &types.QueryResponse{Code: 1, Log: "Não encontrado"}, nil
		}
		return &types.QueryResponse{Code: 0, Value: balanceBytes}, nil
	}
	
	return &types.QueryResponse{Code: 1, Log: "Rota não suportada"}, nil
}

func (app *OrmuzLedgerApp) Commit(ctx context.Context, req *types.CommitRequest) (*types.CommitResponse, error) {
	return &types.CommitResponse{}, nil
}