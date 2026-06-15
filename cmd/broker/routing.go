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

// GetAllMembers retorna todos os IPs conhecidos do anel consistente
func (r *SectorRouter) GetAllMembers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.ring.Members() == nil {
		return []string{}
	}
	return r.ring.Members()
}

// AddMember adiciona um novo membro ao anel
func (r *SectorRouter) AddMember(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ring.Add(ip)
	log.Printf("[ROUTER] Membro adicionado: %s", ip)
}

// RemoveMember remove um membro do anel
func (r *SectorRouter) RemoveMember(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ring.Remove(ip)
	log.Printf("[ROUTER] Membro removido: %s", ip)
}

// ============================================================
// Helper Functions (Roteamento e Network)
// ============================================================

// getLocalIP obtém o IP local da máquina (não-loopback)
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

// isLocalIP verifica se um IP é local (pertence a esta máquina)
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

// forwardPacket encaminha um pacote UDP para outro broker
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
