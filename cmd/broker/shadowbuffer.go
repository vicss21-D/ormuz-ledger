package main

import (
	"log"
	"sync"
	
	"ormuz-ledger/pkg/model"
)

type ShadowBufferManager struct {
	Buffer sync.Map 
}

func NewShadowBufferManager() *ShadowBufferManager {
	return &ShadowBufferManager{}
}

// RescueOrphanedMissions varre o atomicArray e move missões que agora pertencem a este broker
func (sb *ShadowBufferManager) RescueOrphanedMissions() {
	count := 0

	sb.Buffer.Range(func(key, value interface{}) bool {
		mission, ok := value.(model.Mission)
		if !ok {
			return true
		}

		currentOwner := getOwnerIP(mission.Payload.SectorID)

		// Se for responsável do setor, move para heap
		if isLocalIP(currentOwner) {
			missionQueue.Enqueue(mission)
			sb.Buffer.Delete(key)
			count++
			log.Printf("[RESCUE] Missão %s (Setor %d) resgatada da orphan array",
				mission.Payload.EventID, mission.Payload.SectorID)
		}

		return true
	})

	if count > 0 {
		log.Printf("[RESCUE] Total resgatado: %d missões", count)
	}
}

// StoreOrphanedMission armazena uma missão que não pertence a este broker
// Será resgatada quando o broker se tornar o novo dono (failover)
func (sb *ShadowBufferManager) StoreOrphanedMission(mission model.Mission) {
	sb.Buffer.Store(mission.Payload.EventID, mission)

	// Agenda resgate assíncrono (não-bloqueante)
	select {
	case rescueChan <- model.RescueEvent{EventID: mission.Payload.EventID}:
	default:
		// Canal cheio, será resgatado no próximo full scan
	}
}

func (sb *ShadowBufferManager) ClearMission(eventID string) {
	sb.Buffer.Delete(eventID)
	log.Printf("[SHADOW-BUFFER] Evento %s expurgado da memória local.", eventID[:8])
}