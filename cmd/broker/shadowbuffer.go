package main

import (
	"log"
	"sync"

	"ormuz-ledger/pkg/model"
	"ormuz-ledger/pkg/queue"
)

// ShadowBufferManager stores missions that don't belong to this broker.
type ShadowBufferManager struct {
	Buffer sync.Map
}

// NewShadowBufferManager creates a new shadow buffer manager.
func NewShadowBufferManager() *ShadowBufferManager {
	return &ShadowBufferManager{}
}

// RescueOrphanedMissions moves missions that now belong to this broker to the queue.
func (sb *ShadowBufferManager) RescueOrphanedMissions(router *SectorRouter, missionQueue *queue.PriorityQueue) {
	count := 0

	sb.Buffer.Range(func(key, value interface{}) bool {
		mission, ok := value.(model.Mission)
		if !ok {
			return true
		}

		currentOwner := router.GetOwnerIP(mission.Payload.SectorID)

		// Se for responsável do setor, move para heap
		if isLocalIP(currentOwner) {
			missionQueue.Enqueue(mission)
			sb.Buffer.Delete(key)
			count++
			log.Printf("[RESCUE] Missão %s (Setor %d) resgatada da orphan buffer",
				mission.Payload.EventID[:8], mission.Payload.SectorID)
		}

		return true
	})

	if count > 0 {
		log.Printf("[RESCUE] Total resgatado: %d missões", count)
	}
}

// Store archives a mission that doesn't belong to this broker for failover.
func (sb *ShadowBufferManager) Store(mission model.Mission) {
	sb.Buffer.Store(mission.Payload.EventID, mission)
	log.Printf("[SHADOW-BUFFER] Missão %s armazenada (Setor %d)",
		mission.Payload.EventID[:8], mission.Payload.SectorID)
}

// ClearMission removes a mission from local memory.
func (sb *ShadowBufferManager) ClearMission(eventID string) {
	sb.Buffer.Delete(eventID)
	log.Printf("[SHADOW-BUFFER] Evento %s expurgado da memória local.", eventID[:8])
}
