package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type HTTPClient struct {
	client         *http.Client
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	timeout        time.Duration
}

// NewHTTPClient cria cliente HTTP com tratamento de falhas
func NewHTTPClient(timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: timeout,
		},
		maxRetries:     3,
		initialBackoff: 100 * time.Millisecond,
		maxBackoff:     5 * time.Second,
		timeout:        timeout,
	}
}

// PostJSON faz requisição POST com retry exponencial
func (hc *HTTPClient) PostJSON(url string, body interface{}, result interface{}) error {
	var payload []byte
	var err error

	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("erro ao serializar body: %v", err)
		}
	}

	var lastErr error
	backoff := hc.initialBackoff

	for attempt := 0; attempt <= hc.maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("Retry %d/%d em %v para %s", attempt, hc.maxRetries, backoff, url)
			time.Sleep(backoff)
			// Exponential backoff com jitter
			backoff = time.Duration(float64(backoff) * 1.5)
			if backoff > hc.maxBackoff {
				backoff = hc.maxBackoff
			}
		}

		req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
		if err != nil {
			lastErr = fmt.Errorf("erro ao criar requisição: %v", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Strait-of-Ormuz/1.0")

		resp, err := hc.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("erro de conexão: %v", err)
			continue
		}

		// Trata status codes
		if resp.StatusCode >= 500 ||
			resp.StatusCode == http.StatusRequestTimeout ||
			resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusServiceUnavailable {
			// Erro servidor ou rate limit ou timeout: pode fazer retry
			resp.Body.Close()
			lastErr = fmt.Errorf("servidor retornou %d", resp.StatusCode)
			continue
		}

		if resp.StatusCode >= 400 {
			// Erro cliente: não faz retry
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("erro HTTP %d: %s", resp.StatusCode, string(body))
		}

		// Status 2xx: sucesso
		if result != nil {
			if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
				resp.Body.Close()
				return fmt.Errorf("erro ao desserializar resposta: %v", err)
			}
		}

		resp.Body.Close()
		return nil
	}

	return fmt.Errorf("falha após %d tentativas: %v", hc.maxRetries, lastErr)
}

// GetJSON faz requisição GET com retry exponencial
func (hc *HTTPClient) GetJSON(url string, result interface{}) error {
	var lastErr error
	backoff := hc.initialBackoff

	for attempt := 0; attempt <= hc.maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("Retry GET %d/%d em %v para %s", attempt, hc.maxRetries, backoff, url)
			time.Sleep(backoff)
			backoff = time.Duration(float64(backoff) * 1.5)
			if backoff > hc.maxBackoff {
				backoff = hc.maxBackoff
			}
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			lastErr = fmt.Errorf("erro ao criar requisição: %v", err)
			continue
		}

		req.Header.Set("User-Agent", "Strait-of-Ormuz/1.0")

		resp, err := hc.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("erro de conexão: %v", err)
			continue
		}

		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout {
			resp.Body.Close()
			lastErr = fmt.Errorf("servidor retornou %d", resp.StatusCode)
			continue
		}

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("erro HTTP %d: %s", resp.StatusCode, string(body))
		}

		if result != nil {
			if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
				resp.Body.Close()
				return fmt.Errorf("erro ao desserializar resposta: %v", err)
			}
		}

		resp.Body.Close()
		return nil
	}

	return fmt.Errorf("falha após %d tentativas: %v", hc.maxRetries, lastErr)
}

// NewErrorResponse cria resposta de erro
func NewErrorResponse(err string, code string) ErrorResponse {
	return ErrorResponse{
		Error:     err,
		Code:      code,
		Timestamp: time.Now(),
	}
}