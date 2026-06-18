# Ormuz Ledger

## Visão Geral

O **Ormuz Ledger** é uma infraestrutura de Comando e Controle baseada em blockchain de consórcio, desenvolvida para gerir e auditar operações por meio de drones. Utilizando o motor de consenso **CometBFT**, a rede garante tolerância a falhas bizantinas (BFT) e imutabilidade dos dados entre as nações participantes. O sistema atual é uma evolução do projeto **strait-of-ormuz**, adicionando — além da blockchain para registro imutável das operações — um sistema de economia e melhorias nas interfaces e comunicação entre entidades.

---

## Arquitetura

O Ormuz Ledger opera através de uma topologia de rede distribuída onde microsserviços interagem de forma assíncrona com uma malha de consenso BFT (*Byzantine Fault Tolerance*). A comunicação entre as camadas é estritamente dividida entre operações físicas e o registo imutável de estado (Blockchain).

### Componentes

* **CometBFT (Camada de Consenso):** Rede P2P (*Peer-to-Peer*) formada pelos nós validadores (Nações). Recebe as transações dos Brokers, garante que todos os nós concordem com a ordem exata dos eventos e empacota essas transações em blocos criptográficos.
* **Ledger App (Go + LevelDB):** Aplicação em Go que armazena o estado global (LevelDB). Comunica-se com o CometBFT local através do protocolo ABCI (*Application Blockchain Interface*), processando cada bloco validado e alterando os saldos ou registando relatórios.
* **Sensores:** Simula sensores em diferentes áreas que geram dados (podendo ser uma requisição ou não), atribui a requisição a uma nação, com parâmetros que podem simular diferentes níveis de carga.
* **Brokers:** Roteadores econômicos que recebem os pacotes brutos dos sensores, identificam e os ordenam. Recebem solicitações de missões das estações, envelopam as transações com as devidas assinaturas e enviam os RPCs para o CometBFT.
* **Estações:** Interfaces de comando que despacham ordens de missão para os drones.
* **Drones:** Unidades físicas que executam operações nos diferentes setores. Comunicam-se exclusivamente com as Estações ou Brokers para devolver telemetria e resultados de missão em formato JSON.

### Fluxo de Comunicação Assíncrona

#### 1. Autorização da Missão
1. **Estação -> Broker (HTTP):** A Estação despacha uma ordem de missão.
2. **Broker -> CometBFT (JSON-RPC):** O Broker empacota uma transação do tipo `SPEND_CREDIT` (contendo a assinatura da Nação, mas sem payload de dados) e envia via endpoint `/broadcast_tx_sync`.
3. **CometBFT -> Ledger App (ABCI):** O consenso empacota a transação em um bloco e a envia via `FinalizeBlock` para a aplicação.
4. **Mutação de Estado:** O Ledger valida o duplo gasto, reduz o saldo da Nação em 1 crédito no LevelDB e autoriza a continuidade física da missão.

#### 2. Conclusão e Auditoria
1. **Drone -> Broker/Estação (HTTP):** Após finalizar o voo, o Drone envia os dados brutos de telemetria e o resultado (alvo abatido, tempo de voo, etc.).
2. **Broker -> CometBFT (JSON-RPC):** O Broker anexa o JSON do Drone dentro do *Payload* de uma transação do tipo `SAVE_REPORT` e a envia para a rede de consenso usando o mesmo ID da missão original.
3. **CometBFT -> Ledger App (ABCI):** O bloco é validado e processado pela rede.
4. **Mutação de Estado:** O Ledger reconhece o tipo `SAVE_REPORT` e arquiva o payload do Drone de forma imutável no LevelDB, garantindo a auditoria pública da missão sem alterar saldos financeiros.

#### 3. Consulta de Estado
Para auditoria em tempo real, os clientes realizam requisições HTTP (`GET /explorer`) para o Broker. O Broker executa uma chamada `abci_query` no CometBFT, que por sua vez lê o LevelDB e devolve o estado consolidado (Saldos e Histórico de Missões) para renderização.

### Fluxo Geral

1. Sensores geram uma requisição e encaminham para os brokers.
2. Brokers analisam a requisição e solicitam a validação de saldo aos nós do consórcio.
3. Quando aprovada, o broker adiciona a missão à fila de prioridade.
4. Drones livres solicitam à sua estação de registro uma missão.
5. A estação faz a requisição da missão para o broker e a delega para o drone.
6. Drone conclui a missão e envia o relatório diretamente a um nó da blockchain.

---

## Arquitetura Anterior x Arquitetura Atual

O ponto onde a arquitetura de comunicação sofreu a maior transformação foi na interação entre a estação, brokers e drones.

#### Sistema Antigo: Comunicação Altamente Acoplada (*The Lease Pattern*)
* **Fluxo:** `Drone -> Broker` *(A cada 5 segundos)*
* **Protocolo:** HTTP POST (`/mission/renew`)
* **Como era:** Para a missão existir, o Drone e o Broker mantinham uma conexão de rede. O Drone disparava *pings* contínuos (renovação do *Lease*). Se a comunicação de rede falhasse por 15 segundos, o Broker abortava a missão na RAM e a devolvia para a fila de prioridade (*Heap*).

#### Sistema Novo: Comunicação Desacoplada
* **Fluxo:** Quebrado em dois eventos atômicos independentes no tempo.
* **Protocolos:** HTTP POST $\rightarrow$ JSON-RPC $\rightarrow$ ABCI
* **Como ficou:** A introdução da blockchain extinguiu a necessidade do *Lease* (renovação constante).
  1. **Autorização:** A estação emite um `SPEND_CREDIT`. O saldo da nação é reduzido no Ledger e a missão é liberada. A comunicação com o consenso acaba aqui.
  2. **Voo:** O drone atua **100% offline** em relação à rede de consenso. Falhas ou perda de sinal de rede não cancelam o voo físico.
  3. **Auditoria:** Após finalizar a operação, o drone pousa (ou retoma conexão) e emite um `SAVE_REPORT`. O Ledger reconhece a operação e anexa o relatório de forma imutável ao histórico da missão original.
  4. **Renovação:** O broker ainda mapeia um tempo de expiração para cada missão, não vinculado ao lease. Caso o tempo expire sem confirmação, a missão é renovada.
 
O principal ponto aqui é uma maior abstração entre os brokers e os drones, que antes possuíam alto acoplamento, devido à renovação do Lease, e consequentemente tornando a estação menos significativa. No sistema atual, as resposabilidades foram majoritariamente movidas para a estação. Além disso, os drones atuam como _light clients_ que enviam os relatórios diretamente para a blockchain, garantindo que mesmo que a estação tenha sido destruída durante o voô, o relatório da missão tenha sido devidamente registrado.

## Dependências

* **Docker** (Engine v20.10+)
* **Docker Compose** (v2.0+)
* *Opcional:* **Go 1.23.5+** (para desenvolvimento local).

---

## Execução

O ecossistema foi desenhado para ser orquestrado inteiramente via Docker Compose, instanciando múltiplos nós validadores e réplicas de microsserviços automaticamente.

### 1. Inicialização Limpa
Para garantir que a rede inicie a partir do Bloco 0 (sem histórico residual de blocos antigos no CometBFT ou dados no LevelDB), utilize o script de reset fornecido:

**No Windows (PowerShell):**
```powershell
.\reset.ps1
```
**ATENÇÃO:** Caso não consiga executar o arquivo, execute o comando abaixo antes e tente novamente:

```powershell
Set-ExecutionPolicy -ExecutionPolicy Bypass -Scope Process
```
**No Linux:**

```terminal
make reset
```

### 2. Inicializar a Rede (Uma máquina física)

**No Windows (PowerShell):**
```powershell
docker compose up --build
```

**No Linux:**

```terminal
make up
```

**Para verificar outros comandos úteis, utilize:**

```terminal
make help
```

### 3. Visualizar Relatórios

Para acompanhar a execução das missões, a redução dos saldos e a chegada dos relatórios criptográficos, acesse o endpoint de exploração gerido por um dos Brokers ativos.

Acesse no navegador:

http://localhost:8080/explorer (para o primeiro broker)

http://localhost:8081/explorer (para o segundo broker)
