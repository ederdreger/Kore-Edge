# Fase 13 — Integração matriz por mocks

## Escopo e aceite

Contrato API proposto para Kore-Player TV e sincronização de catálogo exercitada
exclusivamente com mocks. O repositório não contém o contrato oficial da matriz;
esta proposta não comprova compatibilidade com instalações existentes.
Consulte [OpenAPI da proposta v1](../contracts/kore-player-tv-v1.openapi.yaml).
Nenhuma matriz, servidor Flussonic ou host de produção é acessado.

## Contrato implementado no mock

GET `/api/v1/edges/{edge_id}/catalog`, sem query/body, com
`Accept: application/vnd.kore-edge.catalog.v1+json`. O identificador aceita
alfanuméricos, hífen e underscore, até 64 caracteres. A URI `https://mock.invalid`
é apenas um identificador interno: o RoundTripper em memória não usa TLS, DNS,
proxy ou sockets. Não há endpoint remoto configurável e `mode: remote` é recusado.
Esse mock não comprova transporte mTLS ou autenticação da matriz.

Resposta 200: Content-Type exato e envelope `{payload, signature}` da Fase 04.
Assinatura Ed25519 cobre `kore-edge/catalog/v1\n` e os bytes exatos do payload
decodificado. Chave pública e parceiro são fixados no YAML local, nunca na resposta.
O manifesto exige versão 1, revisão positiva, parceiro esperado, timestamps válidos,
validade máxima de 24 h, até 512 canais/grants e fontes `synthetic://<id>`.
Permissões continuam limitadas a `udp`/`hls`. JSON ambíguo/extra, assinatura inválida,
manifesto expirado/futuro, outro parceiro, revisão antiga e mesma revisão com bytes
diferentes são recusados. A mesma revisão com os mesmos bytes é idempotente.

O ETag é o SHA-256 dos bytes do envelope, em hexadecimal minúsculo entre aspas.
O worker só guarda um ETag 200 que corresponda aos bytes que acabou de validar;
ETag ausente/divergente não invalida um manifesto assinado válido, mas desabilita
a consulta condicional seguinte. Resposta 304 exige eco exato do ETag aceito e
catálogo já validado ainda dentro da validade. Não renova permissões nem validade.
Redirects, compressão, MIME diferente e outros status são falhas; nunca são seguidos.
O corpo inteiro é limitado a 256 KiB, mesmo sem Content-Length ou com tamanho falso.

## Configuração e ciclo de vida

Desabilitado por padrão. Exemplo comentado em `configs/edge.example.yaml`:

```yaml
integration:
  enabled: true
  mode: mock
  edge_id: edge-lab
  mock_catalog: /absolute/private/laboratory/signed-mock-catalog.json
  interval_seconds: 30
```

Esse caminho é apenas placeholder, não deve ser copiado sem revisão. Exige
`catalog.partner_id/public_key` válidos. Quando provisionamento também está ativo,
o `edge_id` deve coincidir com o configurado nele; isso não equivale à autenticação
de uma identidade registrada. Ainda não há transporte remoto ou vínculo entre
certificado de cliente e a rota do contrato da matriz.

O arquivo privado contém somente um envelope assinado de laboratório e é relido
por requisição. Diretório 0700, arquivo 0600 no Ubuntu, absoluto e externo ao
diretório de execução; execute a partir da raiz do checkout. Symlinks e arquivos
irregulares são recusados. Windows não valida DACLs, conforme a Fase 11.
Não colocar chaves privadas, tokens, política real ou dados de clientes no Git.
Os testes geram material criptográfico sintético em memória e usam arquivos temporários.

O worker inicia junto da API, faz uma consulta imediata e, em sucesso, espera o
intervalo explícito (1–3600 s). Cada request recebe contexto com deadline de 2 s.
O transporte mock faz leitura local síncrona: esse deadline não preempta syscalls
de filesystem bloqueadas; use arquivo regular em disco local, não montagem remota.
Falhas consecutivas esperam 1 s e 2 s; a terceira deixa o worker `exhausted`, sem
novas tentativas. Sucesso zera o contador consecutivo. Encerramento cancela a
espera e aguarda o worker. Não existem goroutines duplicadas de Run, retry manual
por endpoint, restart automático do agente ou alteração do Stream Engine.
Retomar após esgotamento exige reinício explícito e autorizado da instância.

Catálogo válido é mantido em memória nas falhas. Depois de expirar, ele fica
indisponível e o supervisor pode retirar canais não autorizados: disponibilidade
nunca prolonga autorização vencida. Uma atualização válida que retire canais
também pode interrompê-los de acordo com a autorização existente.
Não há persistência de catálogo recebido ou high-water mark entre reinícios.
Proteção contra rollback de revisão é da instância em memória; o arquivo privado
revisado é a fonte inicial de cada execução. Persistência antirreplay e integração
de identidade remota são requisitos pendentes antes de qualquer conexão real.

## Estado e API local

GET/HEAD `/api/v1/integration` é somente leitura e passa pelo RBAC da Fase 11.
viewer/operator/admin podem ler; metrics/enroller não ganham novas permissões.
Campos: `mode`, `state`, `attempts`, `consecutive_failures`, `last_attempt`,
`last_success`, `next_attempt`, `catalog_available` e `revision`. Datas ausentes
são null; revision é zero quando o catálogo não está disponível. Não inclui
paths, bodies, erros de parsing, edge/parceiro, chaves, tokens ou URI.

Estados: `disabled`, `waiting`, `syncing`, `synchronized`, `unavailable`, `backoff`,
`exhausted` e `stopped`. O estado descreve o worker/último resultado; consulte
`catalog_available` para disponibilidade atual. `mode: mock` identifica a origem;
o endpoint não afirma conexão com a matriz. Health/readiness do core não dependem
de sucesso de sincronização. O layout e os assets do dashboard não foram alterados;
o painel continua exibindo dados locais e botão de atualizar consultas de leitura.

Enquanto a integração possui o catálogo, PUT `/api/v1/catalog` por admin autenticado
retorna 409; no modo local sem autenticação, o middleware continua negando escrita
com 403 antes do handler. Sem integração, o contrato anterior de PUT permanece.

## Validação e rollback de laboratório

Nenhuma mudança operacional é necessária para executar a suíte:

```text
go test ./... -count=1
go vet ./...
go test -race ./...
```

Testes cobrem assinatura/revisão/parceiro/tempo, consulta condicional, ETag não
confiável, corpo excedente, MIME/status/redirect, timeout, cancelamento, orçamento
de retries, endpoint protegido, exclusão do autor manual e shutdown integrado.
Uma falha de mock até esgotar retries é testada com um stream sintético ativo;
o stream continua recebendo pacotes com uma única tentativa de processo.

Para eventual ativação manual, antes de mudar a instância: revisar conta não root,
configuração, chave pública, partner/edge IDs, envelope sintético e prazo de validade,
permissões e paths privados separados de qualquer instância ativa. Guardar release/
YAML anterior pelo procedimento da Fase 12. Não inferir NIC/IP/porta, não compartilhar
paths HLS/auditoria/provisionamento nem executar comandos de rede/systemd.
Validação posterior: estado `mode: mock`, revisão esperada, catálogo disponível,
assinatura errada preservando anterior e health/readiness do core. O teste não
autoriza nem exige acesso à matriz. Rollback: selecionar configuração anterior
sem `integration` e reiniciar só após autorização operacional específica; o restart
pode interromper streams. Retomar PUT exige autenticação e função admin.

## Resultados

Validação em 25/10/2026, Windows, Go 1.24.9 portátil:

- `go test ./... -count=1`: última execução completa aprovada. Uma execução anterior
  apresentou falha em `TestConcurrentConsumptionAcrossRegistryInstances`, do
  provisionamento não alterado nesta fase. Dez repetições isoladas passaram, assim
  como a nova suíte completa; a causa da ocorrência não foi isolada e deve ser
  acompanhada na homologação. Não foi ocultada por mudança ou remoção de teste.
- `go vet ./...`: aprovado no Windows e na análise cruzada Linux/amd64.
- `gofmt -l cmd internal`: sem arquivos pendentes.
- `npm.cmd run check` e 11 testes do painel aprovados; sem mudança de assets web.
- `bin/edge-agent-phase13.exe` e `bin/edge-agent-phase13-linux-amd64` compilados;
  testes de integração Linux compilados em `bin/integration-linux.test`.
- `go test -race ./...`: não executou por falta de CGO. Execução nativa Ubuntu,
  permissões POSIX e homologação com contrato oficial continuam pendentes.

A ferramenta Go temporária precisou ser restaurada do ZIP já disponível antes
dos checks finais. Nenhum agente existente foi substituído e nenhuma configuração
de serviço, rede, firewall ou host de produção foi alterada. Não há conexão real
habilitada nesta entrega. A Fase 14 requer autorização.
