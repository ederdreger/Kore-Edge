# Conexão WireGuard entre Kore Play e Edge

O botão de copiar confirma sucesso junto ao texto. Se a permissão do navegador
for negada, mantém a seleção e orienta Ctrl+C. Copiar não aplica rede.

## Fluxo no painel

1. Cadastre no Kore Play o peer com a chave pública e IP exclusivo do Edge.
2. No Kore Play, abra **Criar conexão automática**, confira interface, IP,
   porta e rotas detectadas/propostas. Confirme com sua senha para aplicar a matriz.
3. Gere o código de conexão para aquele peer. Expira em dez minutos; não o envie
   em chamados, logs ou Git. A emissão de outro código substitui o anterior.
4. No Edge, preencha interface e porta UDP locais e keepalive; não são inferidos.
   Abra **Criar conexão automática**, cole o código e confira a origem HTTPS,
   chave pública da matriz e IP atribuído antes de sincronizar com sua senha.
5. Confira o plano local e confirme a ativação com sua senha. Dados já preenchidos
   também podem ser aplicados por **Revisar e ativar dados já salvos**.
6. Verifique os dois painéis. **Aplicado · aguardando handshake** não comprova
   conectividade. **Handshake recente** exige observação real de até 180 segundos.
   Para a matriz, todos os peers precisam de handshake recente para esse estado.

Não há teste automático de mídia ou mudança de firewall/NAT/DNS/encaminhamento.
O endpoint UDP deve ser acessível conforme a rede aprovada pelo operador.
O endpoint HTTPS da matriz exige certificado válido para o Edge; não há bypass TLS.

## Habilitação inicial — somente após revisão operacional

Cada servidor precisa de um auxiliar restrito instalado uma vez. Atualizar o
painel não instala esse serviço automaticamente nem aplica um túnel.
O binário Linux é produzido junto à release Edge, ou em checkout revisado:

```sh
go build -trimpath -o /tmp/edge-wg-bridge ./cmd/edge-wg-bridge
```

Não execute com sudo o build. No servidor da matriz, disponibilize o mesmo
binário compilado para sua arquitetura e o script deste repositório.
Dependências pré-existentes: Ubuntu/Linux com systemd, iproute2 (`ip`, `ss`),
wireguard-tools (`wg`). O habilitador não instala pacotes.

Antes, registre o estado sem expor chaves privadas:

```sh
id
uname -m
command -v ip wg ss
ip -j address show
ip -j route show table all
ip -6 -j route show table all
ip -j rule show
wg show interfaces
ss -H -lun
```

Confirme a conta não-root do processo e o arquivo de estado privado (não imprima
o conteúdo). Para instalações Edge padrão, o arquivo costuma ser
`/var/lib/kore-edge/users/wireguard/identity.json`; para Kore Play guiado,
`/var/lib/koreplay/data/wireguard/state.json`. Confirme no host: caminhos
personalizados devem ser passados explicitamente. O arquivo deve existir, ser
regular 0600 e pertencer à conta informada. O auxiliar lê somente esse arquivo.

Substitua os valores confirmados, primeiro em simulação:

```sh
bash scripts/install-wireguard-bridge.sh --role edge --user CONTA_CONFIRMADA --application-state ARQUIVO_PRIVADO_ABSOLUTO --binary /tmp/edge-wg-bridge --plan
```

Na matriz, use `--role matrix` e sua conta/caminho confirmados. O plano mostra
destinos e rollback. Depois da aprovação operacional específica, o mesmo comando
com `sudo` e `--apply` solicita `HABILITAR`. Recusa destinos já existentes,
serviço conflitante, conta root ou arquivo inadequado. Não use como atualizador.

O serviço root `kore-wg-bridge-edge` ou `kore-wg-bridge-matrix` dispõe somente
das capabilities NET_ADMIN, DAC_READ_SEARCH e CHOWN, com filesystem restrito.
Socket `/run/kore-wg-bridge-ROLE/control.sock`, root:GID confirmado 0660,
autorização adicional pelo UID. Nenhuma porta TCP é aberta. O core não ganha
privilégios. O Node usa o socket matrix; override de caminho é configuração do
servidor `KOREPLAY_WIREGUARD_BRIDGE_SOCKET`, nunca parâmetro do navegador.

Pós-habilitação: conferir serviço ativo e proprietário/permissões do socket;
gerar plano pelo painel; comparar interfaces/rotas com a baseline (a instalação
do auxiliar não cria túnel). Só então revisar e confirmar a aplicação no painel.

## Aplicação, falhas e rollback

O plano detecta interface/porta ocupadas, endereços e rotas sobrepostos, configurações
alteradas desde a revisão. Recusa assumir interfaces preexistentes sem diário de
propriedade. Não altera rota default, firewall, regras de policy routing ou NICs
existentes. Usa `wg syncconf` para preservar peers inalterados, endereço local
host /32 ou /128 e somente rotas privadas explicitadas no plano.

O job é assíncrono, limitado, com intenção durável antes das mudanças. Falha
tenta desfazer a criação inicial ou restaurar a última configuração controlada.
**Desfazer última aplicação** exige senha e confirmação. Não restaura alterações
de terceiros. Se o processo interromper com intenção pendente, bloqueia aplicação
e exige recuperação explícita; rollback não confirmado requer revisão operacional.
Não remova o diário root para contornar conflitos.

Rollback da habilitação: primeiro desfaça pelo painel a aplicação de rede, valide
rotas/interfaces contra a baseline; depois pare/desabilite somente a unidade
`kore-wg-bridge-ROLE.service`, conforme aprovação. Preserve diário e identidade.
Parar o auxiliar sozinho não derruba o túnel nem restaura rotas.
Rollback da release não desfaz a rede. Após reboot, é necessário revisar/reaplicar
no painel; este módulo não configura aplicação automática de túnel no boot.

## Protocolo e validação

Código com token aleatório de 256 bits; só o hash fica persistido na matriz.
Vinculado a um peer, IP, chave da matriz e configuração vigente; máximo 32 códigos.
`POST /api/wireguard/connect/claim` exige posse da chave X25519 do Edge via HMAC,
tem rate limit e corpo limitado. Resposta assinada também é verificada pelo Edge.
Repetição idêntica é idempotente; código expirado, peer alterado ou prova inválida
é recusado. O token não viaja na URL. DNS/redirect/proxy não podem redirecionar o
cliente de pareamento para destinos internos; IP privado literal requer HTTPS válido.

Testes usam chaves e kernel sintéticos, com falhas de comandos e rollback; nenhum
comando de rede real é executado. Build Linux valida compilação, não homologa o
túnel num Ubuntu real. Referência: [WireGuard quickstart](https://www.wireguard.com/quickstart/)
e [wg syncconf](https://git.zx2c4.com/wireguard-tools/tree/src/man/wg.8).
