package ledger

// Transaction representa qualquer operação de alteração de estado na blockchain
type Transaction struct {
	Type      string `json:"type"`       // "SPEND_CREDIT" ou "SAVE_REPORT"
	NationID  string `json:"nation_id"`  // "BR", "UK", "FR", "US"...
	EventID   string `json:"event_id"`   // Referência ao evento tático
	Signature string `json:"signature"`  // Prova criptográfica
	Payload   string `json:"payload"`    // Add later
}