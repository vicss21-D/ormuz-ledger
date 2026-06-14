package clock

import "sync/atomic"

var globalLamportClock uint64

// Tick avança o relógio e retorna o tempo atômico exato
func Tick() uint64 {
	return atomic.AddUint64(&globalLamportClock, 1)
}

// Get retorna o valor atual do relógio sem incrementá-lo
func Get() uint64 {
	return atomic.LoadUint64(&globalLamportClock)
}

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