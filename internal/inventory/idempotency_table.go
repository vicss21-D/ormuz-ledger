package cache

import (
	"sync"
	"time"
)

// Impede que eventos duplicados entrem na Heap
type IdempotencyFilter struct {
	seen sync.Map
	ttl  time.Duration
}

// Cria o filtro e inicia o lixeiro em background
func NewIdempotencyFilter(ttl time.Duration) *IdempotencyFilter {
	f := &IdempotencyFilter{
		ttl: ttl,
	}
	
	go f.cleanupWorker(ttl / 2)
	
	return f
}

// CheckAndAdd retorna TRUE se o evento é NOVO. Retorna FALSE se já foi visto.
func (f *IdempotencyFilter) CheckAndAdd(eventID string) bool {
	// LoadOrStore tenta salvar. 
	// Se 'loaded' for true, significa que a chave JÁ EXISTIA (duplicado).
	_, loaded := f.seen.LoadOrStore(eventID, time.Now().Add(f.ttl).UnixNano())
	
	// Retornamos o inverso de loaded: se não estava lá, é novo (true)
	return !loaded
}

// cleanupWorker varre o mapa periodicamente para liberar memória RAM
func (f *IdempotencyFilter) cleanupWorker(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UnixNano()
		
		// sync.Map.Range itera de forma thread-safe
		f.seen.Range(func(key, value any) bool {
			// type assertion (cast) necessário pois sync.Map usa interface vazia (any)
			expiry := value.(int64)
			
			if now > expiry {
				f.seen.Delete(key)
			}
			return true // Retorna true para continuar a iteração
		})
	}
}