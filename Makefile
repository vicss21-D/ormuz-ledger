.PHONY: help up up-d down logs status reset clean test-intruder

# Comando padrão ao digitar apenas 'make'
default: help

help:
	@echo "==========================================================="
	@echo " Ormuz Ledger - Comandos de Infraestrutura"
	@echo "==========================================================="
	@echo "  make up            - Sobe a rede inteira (recompila automaticamente)"
	@echo "  make up-d          - Sobe a rede em segundo plano (detached)"
	@echo "  make down          - Derruba a rede e para os containers"
	@echo "  make logs          - Exibe os logs de todos os containers em tempo real"
	@echo "  make status        - Lista os containers ativos"
	@echo "  make reset         - Reseta a blockchain para o Bloco 0"
	@echo "  make clean         - Limpeza profunda: remove volumes, redes e imagens órfãs"
	@echo "  make test-intruder - Simula ataque de intrusão com assinatura falsa"
	@echo "==========================================================="

up:
	docker compose up --build

up-d:
	docker compose up --build -d

down:
	docker compose down

status:
	docker compose ps

logs:
	docker compose logs -f

# ---------------------------------------------------------
# ROTINA DE RESET DA BLOCKCHAIN (BLOCO 0)
# ---------------------------------------------------------

reset: down
	@echo "Limpando o estado do LevelDB (Aplicação)..."
	@rm -rf ./deployments/data/*
	@echo "Limpando o histórico de blocos do CometBFT (Consenso)..."
	@rm -rf ./deployments/config/*/data/*
	@echo "Recriando estados zerados para os validadores..."
	@for nation in br fr uk us; do \
		mkdir -p ./deployments/config/$$nation/data; \
		echo '{"height": "0", "round": 0, "step": 0}' > ./deployments/config/$$nation/data/priv_validator_state.json; \
	done
	@echo "✅ Ambiente perfeitamente limpo e pronto para o Bloco 0!"

# ---------------------------------------------------------
# ROTINA DE LIMPEZA PROFUNDA
# ---------------------------------------------------------

clean: down
	@echo "Removendo volumes do Docker..."
	docker compose down -v
	@echo "Limpando imagens e redes não utilizadas..."
	docker system prune -f

# ---------------------------------------------------------
# ROTINA DE TESTE DE SEGURANÇA (PEN-TEST)
# ---------------------------------------------------------

test-intruder:
	@echo "🚨 Simulando ataque de intrusão (Transação Forjada)..."
	@curl -s -X POST "http://localhost:26657/" \
		-H "Content-Type: application/json" \
		-d '{"jsonrpc": "2.0", "id": 1, "method": "broadcast_tx_sync", "params": {"tx": "eyJ0eXBlIjoiU1BFTkRfQ1JFRElUIiwibmF0aW9uX2lkIjoiQlIiLCJldmVudF9pZCI6IkhBQ0tFRC05OTkiLCJzaWduYXR1cmUiOiJiYWRjMGZlZWJhZGMwZmVlYmFkYzBmZWViYWRjMGZlZWJhZGMwZmVlYmFkYzBmZWViYWRjMGZlZWJhZGMwZmVlYmFkYzBmZWViYWRjMGZlZWJhZGMwZmVlYmFkYzBmZWViYWRjMGZlZWJhZGMwZmVlYmFkYzBmZWViYWRjMGZlZSIsInBheWxvYWQiOiIifQ=="}}' > /dev/null
	@echo "✅ Ataque enviado! Verifique os logs do Docker para confirmar o bloqueio da assinatura."