# Evidências da Fase 14

Data: 25/10/2026. Ambiente: Windows/amd64, Go 1.24.9 portátil, laboratório local
com mocks e fontes sintéticas. Checkout sem `.git`; não há commit/branch de evidência.
Relatório compilado a partir das saídas observadas, não é atestado assinado nem
execução no Ubuntu. **Decisão: NO-GO para implantação; aceite humano pendente.**

## Resultados observados

| Verificação | Resultado | Evidência/limite |
| --- | --- | --- |
| Nova suíte de aceitação | PASS | Quatro cenários e helper; `go test ./tests/acceptance -count=1 -timeout=60s -v` |
| Carga HTTP local | PASS | 512/512 respostas corretas, 8 workers; duração observada 16,4994 ms, maior requisição 2,5003 ms; sem SLA sustentado |
| Perda TS simulada | PASS | 40 gerados, 38 entregues, 2 descontinuidades CC, alerta confirmado |
| Recuperação e capacidade | PASS | Canal recuperado: 2 tentativas/1 reinício; saudável: 1 tentativa; limite de 2 ativos aplicado |
| Restart do core auxiliar | PASS | Dois processos próprios; health/readiness e rebind após parada; nenhum serviço externo reiniciado |
| Concorrência de provisionamento | FAIL | 2 de 50 repetições de `TestConcurrentConsumptionAcrossRegistryInstances` falharam; 16 competidores por repetição |
| Regressão Go completa | PASS na passagem posterior | Não anula a falha do ensaio repetido |
| go vet / gofmt | PASS | Vet nativo e análise cruzada Linux; formatação sem pendências |
| Painel TypeScript/testes/build | PASS | 11 testes; build incorporado; avisos Lucide `use client` sem falha |
| HLS sintético real | PASS | Teste de 11,98 s: H.264/AAC decodificados via HTTP, relógio FFplay dummy avançou, expiração/revogação/limpeza passaram |
| Race detector | BLOQUEADO | `-race requires cgo`; não executou |
| Build Linux e testes de aceitação | COMPILADOS | Não executados no Ubuntu |

O HLS real usa o handler de mídia em servidor de teste e ferramentas portáteis já
disponíveis. Não demonstra renderização em navegador ou funcionamento de um
dispositivo com certificados mTLS. Os testes de symlink/permissões POSIX da suíte
não foram convertidos em aprovação Windows. Identificação de origem sintética
não equivale a métricas medidas em NIC física.

## Bloqueio reproduzido

`TestConcurrentConsumptionAcrossRegistryInstances`: duas repetições falharam com
`unexpected enrollment failure`; a primeira delas registrou duas ocorrências e
a segunda, uma. Não houve alteração do código de provisionamento nesta fase.
A mensagem não identifica a causa de baixo nível. Não assumir que é apenas um
efeito do Windows ou que será resolvido no Linux. Investigar e comprovar a correção
com teste de regressão, depois repetir estresse e detector de corrida no alvo.

## Checklist obrigatório de aceite

- [x] Testes sintéticos de aceitação implementados e executados sem rede externa.
- [x] Compilação de candidato Linux e identificação dos artefatos.
- [x] Teste HLS sintético de áudio/vídeo executado com ferramentas existentes.
- [ ] Falha de concorrência do provisionamento investigada e corrigida.
- [ ] Testes completos e repetidos no Ubuntu 24.04, conta não root, sem falhas.
- [ ] Detector de corrida executado com CGO/compilador revisados no ambiente alvo.
- [ ] Permissões POSIX, symlinks e arquivos privados homologados no alvo.
- [ ] Quota/rotação operacional de auditoria, locks após crash e recuperação validados.
- [ ] Carga sustentada/soak com limites acordados, CPU/RAM/latência/bitrates registrados.
- [ ] Multicast na NIC/VLAN isolada, TTL e ausência de vazamento comprovados.
- [ ] HLS/player/mTLS em dispositivos alvo e proxy revisado homologados.
- [ ] Dashboard renderizada e comparada à imagem de referência, com aceite visual.
- [ ] Atualização/rollback e restart do serviço real testados em janela autorizada.
- [ ] Contrato oficial Kore-Player TV, identidade e persistência antirreplay revisados antes de conexão real.
- [ ] Responsável, data, ambiente, release e decisão humana de aceite registrados.

Não há aprovação implícita por completar o desenvolvimento das 14 etapas.

## Artefatos locais

| Arquivo | SHA-256 |
| --- | --- |
| `bin/edge-agent-phase14.exe` | `27e66f88186de340a9f945250c8e6e01a477327fff093fd3e28d5caa271f4e5d` |
| `bin/edge-agent-phase14-linux-amd64` | `54884a7e8af8995b57ff68340001d66165dea10a446022cff2d4922523ae1aab` |

`bin/acceptance-linux.test` foi compilado para execução futura no laboratório.
Hashes identificam estes arquivos, sem assinatura/garantia de origem externa.
Os arquivos em `bin/` não são versionados. Nenhum agente existente, matriz,
Flussonic, configuração de rede, firewall ou serviço instalado foi alterado.
