# WireGuard: credenciais do Edge e peer Kore Play

## Escopo entregue

Menu **WireGuard → Integração WireGuard com Kore Play**. Gera um par X25519
WireGuard do Edge e prepara os dados do peer. Exporta chave pública e bloco
`[Peer]` para cadastro manual no Kore Play, conforme o formato informado pelo
operador. Não pressupõe nem chama uma API proprietária do Kore Play.

Não executa comandos de rede, `wg set`, `wg-quick`, systemd, firewall, DNS ou
conexões à matriz. Salvar não ativa interface nem instala rotas. O estado
`peer_prepared`, sempre com `applied: false`, não significa VPN conectada.
O inventário real anterior permanece separado e pode estar indisponível sem
privilégios; handshake histórico não prova conectividade atual.

## Uso no painel

1. Atualize o sistema pelo procedimento revisado de `update-panel.sh --plan`
   e `--apply`, confirmando endereços e a breve interrupção do painel. Há backup,
   rollback e validação HTTPS/JavaScript. Nenhuma configuração de rede é aplicada.
2. Entre como administrador e abra **WireGuard**.
3. Digite sua senha atual, confirme a geração e clique **Gerar credenciais
   WireGuard**. A geração é idempotente: repetir não troca a chave existente.
4. Copie a **chave pública do Edge** para registrar o equipamento no Kore Play.
5. Obtenha do administrador do Kore Play o IP atribuído ao Edge, chave pública da
   matriz, endpoint UDP e rotas autorizadas. Confirme também o nome da interface
   e a porta UDP do Edge. Não confundir a porta UDP WireGuard com a porta HTTPS.
6. Preencha os campos, confira os dados e confirme com sua senha atual.
7. Copie o **peer público para Kore Play**. Contém a chave do Edge, seu IP de
   túnel como AllowedIPs e, se informado, seu endpoint. Não contém chave privada.

Endpoint do Edge é opcional quando ele iniciará a comunicação atrás de NAT;
o Kore Play poderá aprender o endpoint após tráfego autenticado, quando o túnel
for ativado em etapa operacional aprovada. PersistentKeepalive configurado aqui
é do Edge para o Kore Play, não é copiado para o peer inverso.

IP do Edge deve ser RFC1918/ULA com /32 ou /128. Até 32 rotas AllowedIPs privadas,
canônicas, sem sobreposição e sem conter o IP local. Default routes e redes
multicast/públicas não são aceitas. Endpoints aceitam domínio ASCII ou IP unicast,
privado/público, com porta explícita; loopback, unspecified e IPv4-mapped são
recusados. Domínios não são resolvidos durante preparação. Nenhum endereço,
interface, porta ou keepalive é preenchido como se já tivesse sido detectado.
Dados devem ser conferidos com o inventário e o administrador da rede antes de
ativar um túnel; o formulário não detecta conflitos de rotas no host.

## Segurança e persistência

A API exige sessão HTTPS de administrador. Mutações exigem Origin, CSRF,
JSON estrito de até 8192 bytes, confirmação explícita e senha atual; limites
de verificação e concorrência existentes do painel permanecem. Contas de leitura
não podem consultar nem alterar esta preparação. Ações registram apenas IDs,
tipo de ação, horário e resultado na auditoria privada de contas.

Geração usa `crypto/rand`, scalar clamping e `crypto/ecdh.X25519` do Go. Sem shell
ou ferramenta externa para gerar a identidade. A chave privada fica exclusivamente
no servidor em `<accounts.directory>/wireguard/identity.json`; na instalação
padrão: `/var/lib/kore-edge/users/wireguard/identity.json`. Diretório 0700,
arquivo 0600, gravação com sync/rename, lock exclusivo do sistema operacional.
Windows não substitui validação POSIX/DACL de produção.

A resposta usa DTO sem campo de chave privada. A UI não a recebe nem salva em
localStorage. Não há exportação privada, rotação automática, PSK ou segredo
compartilhado de API nesta versão. Perder esse arquivo significa perder a identidade:
backups precisam ser privados, preservados e nunca enviados a Git ou logs.
Não apagar a base de usuários para atualizar ou recriar a identidade.

Atualizações preservam o arquivo. Configurações usam revisão otimista: alteração
em outra sessão retorna 409 e exige recarregar antes de sobrescrever. Corrupção,
symlink, permissões indevidas ou versão inválida falham fechadas, sem regenerar
automaticamente a chave. A identidade não é regenerada ao editar o peer.

## API

- `GET /api/v1/wireguard/setup`: estado público administrativo.
- `POST /api/v1/wireguard/setup/identity`: `{current_password, confirmed:true}`.
- `PUT /api/v1/wireguard/setup/peer`:
  `{current_password, confirmed:true, revision, settings}`.

Settings: `interface`, `address`, `listen_port`, `edge_endpoint` opcional,
`kore_play_public_key`, `kore_play_endpoint`, `allowed_ips`, `keepalive_seconds`.
Estados: `unconfigured`, `credentials_generated`, `peer_prepared`.
Essas exceções autorizam somente preparação local: sessões não ganham acesso
aos endpoints operacionais de streams, rede ou provisionamento.

## Testes e limites de aceite

Testes usam chaves sintéticas e arquivos temporários externos ao checkout:
persistência, geração concorrente/idempotente, corrupção, lock, validação,
direção do peer, segredo ausente nas respostas, CSRF, senha, viewer/admin,
conflito de revisão e auditoria sem dados do cadastro. Vitest cobre geração,
confirmação, campos sem dados assumidos, cadastro e acesso somente de admin.
CI Ubuntu extrai wireguard-tools em diretório temporário e executa exclusivamente
`wg pubkey` com chave sintética para conferir compatibilidade. Não instala o pacote,
não cria interface nem acessa peers. Testes de transporte e handshake reais
dependem de etapa operacional separada e não foram executados.

Referências: [WireGuard Quick Start](https://www.wireguard.com/quickstart/),
[wg(8)](https://git.zx2c4.com/wireguard-tools/tree/src/man/wg.8),
[crypto/ecdh](https://pkg.go.dev/crypto/ecdh#X25519).
