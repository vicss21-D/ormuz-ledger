package main

import (
	"log"
	"sync"

	"ormuz-ledger/pkg/model"
	"ormuz-ledger/pkg/queue"
)

type ShadowBufferManager struct {
	Buffer sync.Map
}

func NewShadowBufferManager() *ShadowBufferManager {
	return &ShadowBufferManager{}
}

// RescueOrphanedMissions varre o buffer e move missões que agora pertencem a este broker
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

// Store armazena uma missão que não pertence a este broker
// Será resgatada quando o broker se tornar o novo dono (failover)
func (sb *ShadowBufferManager) Store(mission model.Mission) {
	sb.Buffer.Store(mission.Payload.EventID, mission)
	log.Printf("[SHADOW-BUFFER] Missão %s armazenada (Setor %d)",
		mission.Payload.EventID[:8], mission.Payload.SectorID)
}

// ClearMission remove uma missão da memória local
func (sb *ShadowBufferManager) ClearMission(eventID string) {
	sb.Buffer.Delete(eventID)
	log.Printf("[SHADOW-BUFFER] Evento %s expurgado da memória local.", eventID[:8])
}
