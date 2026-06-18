package clock

import "sync/atomic"

var globalLamportClock uint64

// Tick increments the Lamport clock and returns the new value atomically.
func Tick() uint64 {
	return atomic.AddUint64(&globalLamportClock, 1)
}

// Get returns the current Lamport clock value without incrementing.
func Get() uint64 {
	return atomic.LoadUint64(&globalLamportClock)
}

// Sync synchronizes the local clock with a received clock value using compare-and-swap.
func Sync(receivedClock uint64) uint64 {
	for {

		// 1. Registra o valor atual do relógio
		current := atomic.LoadUint64(&globalLamportClock)

		// 2. Calcula qual deveria ser o novo valor
		max := current
		if receivedClock > max {
			max = receivedClock
		}
		newClock := max + 1

		// 3. Compare-And-Swap: "A CPU só atualiza para newClock se o
		// valor na memória ainda for igual à 'foto' (current) que eu tirei."
		// Se outra thread mudou o valor no meio do caminho, o CAS retorna false,
		// o loop reinicia e tenta de novo sem travar o sistema.
		if atomic.CompareAndSwapUint64(&globalLamportClock, current, newClock) {
			return newClock
		}
	}
}
