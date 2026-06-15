package main

import (
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/stathat/consistent"
)

// SectorRouter gerencia a topologia da rede de brokers
type SectorRouter struct {
	ring               *consistent.Consistent
	mu                 sync.RWMutex
	previousSwarmState string
	sectorCount        int
	dnsTarget          string
}

func NewSectorRouter(sectorCount int, dnsTarget string) *SectorRouter {
	return &SectorRouter{
		ring:        consistent.New(),
		sectorCount: sectorCount,
		dnsTarget:   dnsTarget,
	}
}

// WatchTopology monitora alterações no Docker Swarm e rebalanceia o anel
func (r *SectorRouter) WatchTopology(onTopologyChange func()) {
	for {
		ips, err := net.LookupIP(r.dnsTarget)
		if err == nil && len(ips) > 0 {
			var ipStrs []string
			for _, ip := range ips {
				ipStrs = append(ipStrs, ip.String())
			}
			sort.Strings(ipStrs)
			currentState := strings.Join(ipStrs, ",")

			if currentState != r.previousSwarmState {
				r.mu.Lock()
				r.ring = consistent.New()
				r.ring.NumberOfReplicas = 256
				r.ring.Set(ipStrs)
				r.previousSwarmState = currentState
				r.mu.Unlock()

				log.Printf("[ROUTER] Nova topologia detectada: %s", currentState)
				
				// Gatilho (Callback) para o Broker resgatar missões órfãs
				if onTopologyChange != nil {
					go onTopologyChange()
				}
			}
		}
		time.Sleep(5 * time.Second)
	}
}

// GetOwnerIP retorna quem é o primário responsável pelo setor
func (r *SectorRouter) GetOwnerIP(sectorID int) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("sector-%d", sectorID)
	ownerIP, err := r.ring.Get(key)
	if err != nil {
		return ""
	}
	return ownerIP
}