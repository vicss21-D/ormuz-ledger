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

// SectorRouter manages broker topology and sector ownership using consistent hashing.
type SectorRouter struct {
	ring               *consistent.Consistent
	mu                 sync.RWMutex
	previousSwarmState string
	sectorCount        int
	dnsTarget          string
}

// NewSectorRouter creates a new router for sector management.
func NewSectorRouter(sectorCount int, dnsTarget string) *SectorRouter {
	return &SectorRouter{
		ring:        consistent.New(),
		sectorCount: sectorCount,
		dnsTarget:   dnsTarget,
	}
}

// WatchTopology monitors Docker Swarm topology changes and rebalances the ring.
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

// GetOwnerIP returns the primary broker for a sector.
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

// GetAllMembers returns all known broker IPs in the consistent ring.
func (r *SectorRouter) GetAllMembers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.ring.Members() == nil {
		return []string{}
	}
	return r.ring.Members()
}

// AddMember adds a new broker to the ring.
func (r *SectorRouter) AddMember(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ring.Add(ip)
	log.Printf("[ROUTER] Membro adicionado: %s", ip)
}

// RemoveMember removes a broker from the ring.
func (r *SectorRouter) RemoveMember(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ring.Remove(ip)
	log.Printf("[ROUTER] Membro removido: %s", ip)
}

// ============================================================
// Helper Functions (Routing and Network)
// ============================================================

// getLocalIP retrieves the local non-loopback IP address.
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	for _, addr := range addrs {
		// Obtém a rede IP do endereço
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			// Retorna apenas IPv4
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

// isLocalIP checks if an IP address belongs to the local machine.
func isLocalIP(ip string) bool {
	if ip == "127.0.0.1" || ip == "localhost" {
		return true
	}

	localIP := getLocalIP()
	if ip == localIP {
		return true
	}

	// Verifica contra todas as interfaces
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.String() == ip {
				return true
			}
		}
	}

	return false
}

// forwardPacket sends a UDP packet to another broker.
func forwardPacket(targetIP string, payload []byte) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:9000", targetIP))
	if err != nil {
		log.Printf("[FORWARD] Erro ao resolver endereço %s: %v", targetIP, err)
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Printf("[FORWARD] Erro ao conectar em %s: %v", targetIP, err)
		return
	}
	defer conn.Close()

	_, err = conn.Write(payload)
	if err != nil {
		log.Printf("[FORWARD] Erro ao enviar pacote para %s: %v", targetIP, err)
		return
	}

	log.Printf("[FORWARD] Pacote encaminhado para %s (%d bytes)", targetIP, len(payload))
}
