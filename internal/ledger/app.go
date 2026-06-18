package ledger

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"ormuz-ledger/internal/domain/ledger"

	"github.com/cometbft/cometbft/abci/types"
	"github.com/syndtr/goleveldb/leveldb"
)

var AuthorizedKeys = make(map[string]string)

// OrmuzLedgerApp implements the blockchain state machine with persistent storage.
type OrmuzLedgerApp struct {
	types.BaseApplication

	// Estado Persistente (Substitui os Maps em memória)
	db *leveldb.DB
}

func init() {
	nations := []string{"BR", "FR", "UK", "US"}
	for _, n := range nations {
		// Usa a mesma seed secreta do Broker para calcular a contraparte Pública
		seedString := fmt.Sprintf("%-32s", n+"-SECRET-SEED-ORMUZ-2026")
		seed := []byte(seedString)[:32]

		priv := ed25519.NewKeyFromSeed(seed)
		pub := priv.Public().(ed25519.PublicKey)

		AuthorizedKeys[n] = hex.EncodeToString(pub)
	}
	log.Println("🔐 Chaves Públicas do Consórcio carregadas com sucesso.")
}

// NewOrmuzLedgerApp initializes the ledger application with persistent database.
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

// Close closes the database connection on shutdown.
func (app *OrmuzLedgerApp) Close() {
	app.db.Close()
}

// CheckTx validates a transaction before consensus (mempool phase).
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

// FinalizeBlock applies finalized transactions and updates the state permanently.
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
			log.Printf("Operação Duplicada rejeitada! Tipo: %s | Evento: %s", tx.Type, tx.EventID[:8])
			txResults = append(txResults, &types.ExecTxResult{Code: 4, Log: "Transação já processada"})
			continue
		}

		// 2. MÁQUINA DE ESTADOS (Responsabilidade Única)
		switch tx.Type {
		case "SPEND_CREDIT":
			// Apenas altera o saldo
			balanceKey := []byte(fmt.Sprintf("balance:%s", tx.NationID))
			balanceBytes, _ := app.db.Get(balanceKey, nil)
			balance, _ := strconv.Atoi(string(balanceBytes))

			newBalance := balance - 1
			app.db.Put(balanceKey, []byte(strconv.Itoa(newBalance)), nil)
			log.Printf("[LEDGER] Crédito debitado de %s. Saldo: %d (Evento: %s)", tx.NationID, newBalance, tx.EventID[:8])

		case "SAVE_REPORT":
			// Apenas regista a passagem documental no log
			log.Printf("[LEDGER] Relatório físico validado. (Evento: %s)", tx.EventID[:8])
		}

		// 3. APLICA A TRAVA ESPECÍFICA E GRAVA NO EXPLORER
		app.db.Put(processedKey, []byte("true"), nil)

		// Usamos o tx.Type no nome da chave para que o Débito e o Relatório coexistam pacificamente no Explorer
		explorerKey := []byte(fmt.Sprintf("report:%s_%s", tx.Type, tx.EventID))
		app.db.Put(explorerKey, txBytes, nil)

		txResults = append(txResults, &types.ExecTxResult{Code: 0})
	}

	return &types.FinalizeBlockResponse{
		TxResults: txResults,
		AppHash:   []byte{},
	}, nil
}

// Query retrieves state information from the ledger.
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

// Commit signals the end of a block (state finalization).
func (app *OrmuzLedgerApp) Commit(ctx context.Context, req *types.CommitRequest) (*types.CommitResponse, error) {
	return &types.CommitResponse{}, nil
}
