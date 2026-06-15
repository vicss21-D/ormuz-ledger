package main

import (
	"log"
	"sync"
	"time"

	"ormuz-ledger/pkg/model"
)

// UnverifiedRecord guarda a missão e o seu TTL
type UnverifiedRecord struct {
	Mission   model.Mission
	ExpiresAt time.Time
}

// UnverifiedBufferManager é a Sala de Espera para missões não validadas
type UnverifiedBufferManager struct {
	buffer sync.Map
}

func NewUnverifiedBufferManager() *UnverifiedBufferManager {
	bm := &UnverifiedBufferManager{}
	go bm.startJanitor()
	return bm
}

func (bm *UnverifiedBufferManager) Store(mission model.Mission) {
	// Guarda a missão com um timeout de 10 segundos para a validação do Ledger
	record := UnverifiedRecord{
		Mission:   mission,
		ExpiresAt: time.Now().Add(10 * time.Second),
	}
	bm.buffer.Store(mission.Payload.EventID, record)
}

func (bm *UnverifiedBufferManager) RetrieveAndRemove(eventID string) (model.Mission, bool) {
	if val, ok := bm.buffer.LoadAndDelete(eventID); ok {
		record := val.(UnverifiedRecord)
		return record.Mission, true
	}
	return model.Mission{}, false
}

// startJanitor limpa a memória de missões que nunca receberam resposta do Primário
func (bm *UnverifiedBufferManager) startJanitor() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		now := time.Now()
		bm.buffer.Range(func(key, value interface{}) bool {
			record := value.(UnverifiedRecord)
			if now.After(record.ExpiresAt) {
				bm.buffer.Delete(key)
				log.Printf("[SALA DE ESPERA] Missão %s expirou sem validação (NACK implicito)", key.(string)[:8])
			}
			return true
		})
	}
}