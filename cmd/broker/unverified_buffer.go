package main

import (
	"log"
	"sync"
	"time"

	"ormuz-ledger/pkg/model"
)

// UnverifiedRecord holds a mission and its validation timeout.
type UnverifiedRecord struct {
	Mission   model.Mission
	ExpiresAt time.Time
}

// UnverifiedBufferManager is a waiting room for unvalidated missions.
type UnverifiedBufferManager struct {
	buffer sync.Map
}

// NewUnverifiedBufferManager creates a buffer manager and starts the cleanup worker.
func NewUnverifiedBufferManager() *UnverifiedBufferManager {
	bm := &UnverifiedBufferManager{}
	go bm.startJanitor()
	return bm
}

// Store adds a mission to the waiting room with a validation timeout.
func (bm *UnverifiedBufferManager) Store(mission model.Mission) {
	// Guarda a missão com um timeout de 10 segundos para a validação do Ledger
	record := UnverifiedRecord{
		Mission:   mission,
		ExpiresAt: time.Now().Add(10 * time.Second),
	}
	bm.buffer.Store(mission.Payload.EventID, record)
}

// RetrieveAndRemove gets and removes a mission from the buffer.
func (bm *UnverifiedBufferManager) RetrieveAndRemove(eventID string) (model.Mission, bool) {
	if val, ok := bm.buffer.LoadAndDelete(eventID); ok {
		record := val.(UnverifiedRecord)
		return record.Mission, true
	}
	return model.Mission{}, false
}

// startJanitor removes expired missions that never received validation.
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
