# Arquitetura técnica

## Recepção real compartilhada

`internal/live` liga o catálogo do parceiro às saídas reais, sem alterar o
contrato sintético do Stream Engine antigo. Configurações ficam privadas fora
do checkout. Verificar destino não abre mídia. Salvar em canal ativo valida antes e reinicia
apenas a recepção alterada. Início/parada pelo painel exigem sessão
administrativa, CSRF, auditoria e geração vigente, sem confirmação adicional. Não pede novamente senha para operar canais. Há uma origem por
canal e fan-out limitado para prévias, UDP e segmentador HLS. Revogação encerra
o fluxo; a lease assinada não é estendida por falhas de sincronização.

SRT usa Go puro com AES e segredo por parceiro derivado da identidade X25519.
A matriz escuta somente no IP WireGuard revisado, valida IP do peer, stream ID,
permissões e cotas. O Edge verifica endereço da interface e seleção de origem
antes e depois do handshake. A biblioteca não recebe URLs/segredos em argv.
HLS é remuxado por relay loopback privado, com master playlist em stdin para
permitir atualização da playlist de mídia sem expor a capacidade em argv.

UDP/HLS exigem grants separados; codecs H.264/AAC são verificados antes do
segmentador. Prévias e HLS local mantêm autenticação. Limites e operação:
[distribuição real](modules/19-live-distribution.md).

## Distribuição autorizada Kore Play → Edge

O módulo real `edgecatalog` consulta pelo túnel um catálogo por parceiro,
autenticado pela identidade X25519 existente e HTTPS com certificado validado.
A central conserva as origens e autoriza cada solicitação de mídia. O Edge
expõe apenas entradas locais autenticadas HTTP MPEG-TS/HLS já existentes na
origem; não conecta este catálogo ao Stream Engine sintético nem cria saídas
multicast. Validade, revogação e testes amostrais são explícitos no painel.
O mini player usa somente o proxy autenticado local, com HLS.js ou mpegts.js
carregados sob demanda e sem workers. A CSP permite mídia local/blob e mantém
conexões restritas à própria origem. Prévia única limitada a cinco minutos, com
liberação de recursos ao fechar, trocar, pausar ou expirar a autorização.
Veja [contrato e limites](modules/18-partner-channels.md).

```text
Kore Headend / Control Plane
  | HTTPS (catálogo, configuração, telemetria)
  | WireGuard: transporte privado de MPEG-TS UDP unicast
  v
Kore Edge Ubuntu 24.04
  |- Edge Agent (Go): config, lifecycle, health, sync
  |- Stream Engine (FFmpeg/TSDuck): ingest e fan-out
  |- UDP multicast -> NIC/VLAN IPTV explícita -> moduladores / rede local
  |- HLS -> Nginx -> players autorizados
  |- Metrics + logs -> painel e matriz
```

## Contratos
- Control plane não deve interromper o data plane em perda de conectividade; conservar última configuração válida.
- Manifestos são versionados, validados e aplicados de modo atômico com rollback.
- Por parceiro: identidade VPN, ACL, catálogo autorizado e quotas de banda/sessões.
- Ingest por canal preferencialmente único; fan-out para UDP e HLS.
- HLS exige checagem de codecs, armazenamento temporário, limpeza e proteção de acesso.
- Multicast local usa interface de saída explícita e TTL limitado; não atravessa WireGuard diretamente.
- Monitoramento inclui RX/TX físico e VPN, bitrate por canal, continuidade TS, TEI, PCR quando suportado.
- Gestão e tráfego de mídia segregados por NIC ou VLAN quando disponível.

## Laboratório
Usar fonte sintética, loopback e rede isolada; os endereços 10.250.0.0/24 e 239.18.10.0/24 são apenas exemplos e requerem validação de conflitos.

## Segurança implementada no laboratório — Fase 11

O plano de controle permanece em loopback. O modo padrão permite somente leitura;
mutações exigem HTTPS direto com mTLS/TLS 1.3 e RBAC por fingerprint cadastrado.
A política é reavaliada por requisição e intenções administrativas são auditadas
antes dos efeitos. Configuração de segurança inválida falha fechada; isso não
altera o contrato de conservar o último catálogo válido do data plane.
Segredos e auditoria ficam em diretórios privados externos ao checkout.
Não há confiança implícita em proxies ou autenticação por headers encaminhados.
Veja [Fase 11](modules/11-security.md) para funções, configuração, limites e testes.

## Operação implementada no laboratório — Fase 12

Releases são preparados offline, com binário e cópia exata da configuração em
diretório privado. Seleção e rollback usam uma geração para impedir sobrescrita
por comandos desatualizados. O agente lê a seleção somente na inicialização e
confere integridade e correspondência com o executável iniciado. Não há hot-swap,
download, reinício automático nem alteração de serviços do host. Uma falha na
preparação não altera a seleção anterior nem chama o Stream Engine.
O supervisor mantém orçamento finito de tentativas por canal, espera cancelável
e backoff exponencial limitado a 30 segundos. Alertas locais identificam backoff,
falha definitiva e perda de autorização. Veja [Fase 12](modules/12-operations.md).

## Integração por mocks — Fase 13

Um worker opcional consulta o contrato proposto de catálogo Kore-Player TV por
transporte HTTP em memória, sem DNS, socket ou endereço de matriz configurável.
O catálogo assinado existente aplica atualizações válidas de modo atômico e
mantém o anterior em falhas, sem estender sua validade nem suas permissões.
O worker não controla processos de mídia e para após três falhas consecutivas.
Na presença dele, o PUT manual de catálogo é recusado para manter um único autor.
O endpoint local de estado continua sob RBAC e não expõe credenciais ou paths.
O contrato não foi validado contra uma API real; detalhes e limites estão na
[Fase 13](modules/13-integration.md).

## Homologação — Fase 14

A suíte `tests/acceptance` exerce cenários limitados em pipes sintéticos e loopback:
carga HTTP, omissões TS, recuperação de canal, limite de capacidade e restart de
processos auxiliares próprios. Não altera rede nem serviços existentes. Resultados
locais não certificam o data plane físico ou a compatibilidade com a matriz.
O [checklist de homologação](acceptance/phase14-report.md) registra NO-GO enquanto
existirem falhas de estabilidade, validações obrigatórias pendentes ou ausência
de aprovação humana do laboratório. Nenhum teste aprova uma implantação sozinho.

## Entrada HTTPS da VPS

O assistente aceita domínio/IP da VPS e prepara um gateway Nginx HTTPS com senha
para leitura do painel. A API permanece em loopback/read-only, sem identidade por
headers e sem mutações pelo gateway. Certificados locais são de laboratório;
não há ACME automático nem alteração de firewall/DNS. Regras e rollback em
[INSTALL](INSTALL.md). Isso não equivale a homologação para produção ou integração
de mídia com a matriz.


## Contas locais e painel limpo

Preparação WireGuard autenticada: os endpoints `/api/v1/wireguard/setup*` exigem
administrador, CSRF e confirmação de senha para geração/alteração. Persistem a
identidade X25519 e proposta de peer em subdiretório privado de accounts, sem
executar ações do sistema. A chave privada não faz parte dos DTOs. O bloco de
peer exportado é público e o inventário real continua separado. Contrato em
[credenciais WireGuard](modules/16-wireguard-credentials.md).

Accounts envolve o guard HTTP existente, autentica sessões HTTPS e intercepta
auth/users; preparação WireGuard usa autorização administrativa específica.
Nginx preserva Origin; cookies Secure/HttpOnly e CSRF protegem
mutações. Papéis admin/viewer não concedem escrita nas APIs operacionais.
Base privada, hashes bcrypt, revisões para revogação, auditoria sem segredos e
locks exclusivos. Perfil production exige contas e bloqueia módulos sintéticos/mock.
Veja [contrato e migração](modules/15-accounts.md).


## Aplicação WireGuard explícita

O módulo opcional de conexão usa `edge-wg-bridge`, processo separado do core,
com socket UNIX autenticado por UID (`SO_PEERCRED`). O core permanece sem
CAP_NET_ADMIN. Somente o auxiliar instalado explicitamente pode aplicar a interface
confirmada e as rotas privadas revisadas. Diário root registra propriedade e
rollback, sem chaves privadas. Uma falha interrompida exige recuperação explícita.
Código temporário HTTPS sincroniza dados públicos com prova X25519/HMAC; nenhum
endpoint público fornece comandos, caminhos ou chaves privadas ao auxiliar.
Contrato e limites em [conexão WireGuard](modules/17-wireguard-connection.md).


## Entradas manuais por URL

`internal/manualchannels` implementa HTTP MPEG-TS, HLS, UDP MPEG-TS e SRT caller
com persistência privada e inventário sem credenciais. O adaptador combina somente
a apresentação dos catálogos: autorização dos canais importados segue delegada
à central, sem estender grants ou leases. Entradas locais usam `local_` e são
geridas pela conta administrativa autenticada. A mesma instância de `live` faz
recepção contínua e fan-out para player/UDP/HLS. `ChannelChange` serializa
operações, revalida a conta e audita sem reentrada de senha, preservando CSRF.
Origem e ferramentas não entram em logs/argv; HLS passa por relay loopback privado.
Detalhes e limites em [cadastro manual](modules/20-manual-channels.md).

O reconciliador de recepção Kore Play executa no agente a cada cinco segundos,
com fila, limite de 16 canais ativos e pausa manual persistida em `auto_paused`.
Não estende grants. Quedas são recuperadas com backoff cancelável de até 30 segundos.

`internal/mediaresources` reserva duas entradas HLS e duas saídas/validações
compartilhadas. Falta de capacidade é fila, não falha da origem. A reserva dura
até o término do processo ou fechamento da saída; cancelamentos liberam vagas.
O runtime padrão é limitado no binário sem mudar unidades systemd.

## Saídas permanentes por canal

`internal/playback` deriva uma chave HMAC-SHA256 determinística por identidade e
geração de exclusão do canal. Não depende de conta, sessão ou horário. A chave
raiz e gerações ficam privadas em `users/playback/playback.json`; URLs completas
não são gravadas. GET administrativo lista os endereços automaticamente.
A rota de mídia valida chave e grants atuais, sem permitir administrar o Edge.
O formato temporário anterior continua aceito conforme suas regras originais;
novos endereços usam o domínio criptográfico `permanent-channel-v1`.

TS persistente segue trocas de hub/job sem desconectar por uma tentativa de
reconexão. Pacotes nulos de transporte durante o intervalo não entram no hub nem
nas métricas da origem. Pausa explícita, exclusão, grant indisponível, shutdown,
cliente desconectado e limites de escrita encerram a resposta. Não abre origem
extra. HLS valida cada playlist/segmento com o mesmo endereço permanente; a
existência do endereço não significa que o segmentador está pronto.
