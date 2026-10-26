# Fase 03 — WireGuard

> Evolução posterior: a geração persistente de credenciais e o formulário
> autenticado de peer foram adicionados no [módulo de credenciais](16-wireguard-credentials.md).
> As limitações de geração/persistência descritas abaixo registram a Fase 03
> original. Aplicação de túnel e alterações de rede continuam fora do módulo.

## Entrega e uso local

`internal/wireguard` separa planejamento puro de inventário de leitura. Não há
aplicador, executor de rollback operacional, geração de chaves ou persistência.
Nenhum comando altera o host, nenhuma rota é instalada e nenhum túnel é criado.

- `GET /api/v1/wireguard`: timestamp UTC, origem, estado `real`/`unavailable`
  e peers com interface, chave pública, allowed IPs e último handshake.
  Epoch zero significa `never` e data nula; `observed` informa uma data histórica,
  sem afirmar conexão atual. Sem peers retorna lista vazia, não erro.
- Linux executa exclusivamente `wg show all latest-handshakes` e
  `wg show all allowed-ips`, com orçamento total de dois segundos e limite de
  1 MiB por saída. Não usa `dump`, `showconf`, chaves privadas ou PSKs.
  stderr não é registrado. Sem ferramenta, permissão ou saída válida: indisponível.
- Duas leituras não são transação do kernel. Divergência na lista de peers
  retorna indisponível; alterações apenas de allowed IPs entre consultas ainda
  podem ocorrer. Não usar como baseline operacional completo.
- Windows: indisponível; nunca substitui por inventário fictício.
- `POST /api/v1/wireguard/plan`: recebe `before` (objeto anterior ou null) e
  `desired`, retorna `mode: dry_run`, `applied: false`, template público,
  rotas propostas, pré-condições, validação e ação de rollback.
  JSON limitado a 64 KiB, campos desconhecidos e documentos extras rejeitados.
  Erros retornam 400 sem ecoar valores enviados. GET no planejamento: 405.

O exemplo `configs/wireguard-plan.example.json` é totalmente sintético: chave
pública ilustrativa sem peer correspondente; endereços/porta/interface são
exemplos não aprovados para aplicação. Não envie segredos em nenhum campo.
O planejador exige interface explícita (até 15 caracteres), porta válida,
endereço RFC1918/ULA e peers com chave pública base64 de 32 bytes, endpoint
literal privado/loopback e CIDRs canônicos privados. Rejeita default routes,
rotas públicas/multicast, sobreposições, duplicação de peers e rotas que incluem
o endereço local. Limites: 128 peers, 128 rotas por peer. Não comprova posse da
chave, conflitos com LAN, reachability, nem se um endereço privado é laboratório.

O template usa `Table = off`, não inclui hooks, DNS, chave privada ou PSK e é
deliberadamente incompleto para aplicação. `Table = off` impede criação
automática de rotas pelo wg-quick; não autoriza executar o template.
Referências primárias: [wg(8), fonte WireGuard](https://git.zx2c4.com/wireguard-tools/tree/src/man/wg.8)
e [wg-quick(8), fonte WireGuard](https://git.zx2c4.com/wireguard-tools/tree/src/man/wg-quick.8).

Após revisão do endereço local já confirmado e execução do novo binário:

```sh
curl --fail http://127.0.0.1:18080/api/v1/wireguard
curl --fail -H 'Content-Type: application/json' --data-binary @configs/wireguard-plan.example.json http://127.0.0.1:18080/api/v1/wireguard/plan
```

## Pré-verificação, rollback e pós-validação

Nesta entrega a validação é de planejamento, sem mudança operacional. O plano
não sabe restaurar automaticamente um host nem guarda segredos.

Antes de futura aplicação humana: confirmar host isolado, interface/endereços/
portas/peers, obter aprovação específica e registrar baseline completo de
interfaces, rotas, policy rules, defaults, estado de ativação e configuração
anterior. Guardar material secreto separadamente e protegido. Consultas públicas
possíveis: `wg show interfaces`, `wg show all latest-handshakes`,
`wg show all allowed-ips`; não são comandos de aplicação. Inspeção adicional de
rede e comandos operacionais concretos serão apresentados para aprovação com
os dados reais do laboratório, antes de executá-los.

Com `before`, rollback é o template anterior validado para a mesma interface,
mais restauração exata da baseline de rotas e estado original. Sem `before`,
o plano assume criação nova: o operador deve confirmar que a interface não
existe; rollback remove somente interface/rotas criadas pela mudança aprovada.
Não é seguro usar `before: null` para interface existente. Templates públicos
não recuperam chaves privadas nem PSKs: preservar esse material fora da API.
Após aplicação/rollback futuros, comparar defaults/policy rules com baseline,
conferir handshake e rotas específicas e verificar interfaces/streams alheios.

## Testes e resultado — 2026-10-25

Testes usam executor mockado e exemplo sintético: dry-run determinístico,
rollback de criação e restauração ida/volta, rejeição de defaults (inclusive
divididos), sobreposições, input inválido, contrato HTTP, handshake nunca/observado,
ausência de peers, falha de comando e sanitização de erros.

Go portátil 1.24.9 no Windows: `go test ./...`, `go vet ./...` e gofmt passaram.
Builds Windows/Linux amd64, vet Linux e compilação do teste WireGuard Linux
passaram. `go test -race ./...` bloqueado pelo ambiente sem CGO/compilador C.
WireGuard real e rollback operacional não foram executados: dependem de
laboratório Ubuntu e aprovação específica. O aceite de dry-run/rollback aqui
é de planos testados, não de alteração/restauração de um túnel real.
A instância local anterior não foi substituída. Pasta sem `.git`: branch não criada.

## Objetivo
Inventário de peers, configuração gerada, handshake, rotas permitidas.

## Pré-requisitos
Fase anterior aprovada, `AGENTS.md` lido, laboratório isolado.

## Entregáveis
- Implementação modular e documentada.
- Testes automatizados e exemplos de configuração sem segredos.
- Procedimento de validação e rollback, quando aplicável.

## Critério de aceite
Dry-run e rollback testados; sem alterar rotas default.

## Prompt para Codex
Leia `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` e este documento. Desenvolva somente a Fase 03 (WireGuard): Inventário de peers, configuração gerada, handshake, rotas permitidas. Antes de editar, apresente um plano, dependências, testes e riscos. Não execute mudanças privilegiadas no host. Entregue código, testes e documentação. Demonstre como validar: Dry-run e rollback testados; sem alterar rotas default. Ao terminar, pare e solicite aprovação para a próxima fase.
