# Instalador de laboratório — Ubuntu Server 24.04

**Quer instalar respondendo perguntas?** Execute `bash scripts/setup.sh` e siga
o [passo a passo simples](INSTALACAO-SIMPLES.md). Sem argumentos em um terminal,
`bash scripts/install.sh` também abre esse assistente. Ele mostra o plano antes
da escolha final e não instala sem confirmação. O restante deste documento é a
referência técnica do modo por parâmetros.

O sistema e o instalador são públicos em
[ederdreger/Kore-Edge](https://github.com/ederdreger/Kore-Edge). O estado de
homologação permanece **NO-GO para produção**: há uma falha de concorrência
reproduzida no provisionamento e validações físicas pendentes. Este instalador
não substitui o [checklist de aceite](acceptance/phase14-report.md).

## Conteúdo e pré-requisitos

Host Ubuntu 24.04 com systemd, apt/dpkg, Bash, ferramentas básicas e checkout Git
limpo de um commit revisado. Git é pré-requisito para obter/verificar o checkout;
não é suportado executar um script isolado baixado via `curl | sudo bash`.
Arquitetura nativa suportada por Go/pacotes APT; não há cross-compilation no instalador.
O instalador usa os repositórios Ubuntu já configurados e não adiciona PPA/chave
de terceiros, não executa upgrade completo nem configura repositórios de mídia.
Se um pacote não tiver candidato nos repositórios disponíveis, a instalação falha
em vez de adicionar fontes automaticamente.

| Perfil | Pacotes Ubuntu |
| --- | --- |
| core | ca-certificates, curl, git, golang-go, build-essential, iproute2 |
| media (padrão) | Todos do core + ffmpeg, wireguard-tools |

Go compila o sistema, build-essential permite o detector de corrida, FFmpeg inclui
FFprobe para HLS, e wireguard-tools disponibiliza o inventário `wg`. Instalar a
ferramenta não concede NET_ADMIN nem configura peers. O inventário pode permanecer
indisponível sem privilégios; isso não é mascarado como conexão ativa.
Dashboard já compilada é incorporada no Go: Node/npm não são necessários no
servidor. Não são instalados Nginx ou TSDuck: o código atual serve HTTP/HLS em Go
e não depende de TSDuck. Proxy ou rede de mídia exigem projeto/validação separados.

Fontes de referência: [pacotes Ubuntu](https://packages.ubuntu.com/),
[GitHub CLI](https://cli.github.com/manual/gh_repo_create) e
[setup-go](https://github.com/actions/setup-go). As versões APT vêm do host Ubuntu;
a CLI não instala distribuições Go externas ou binários FFmpeg de terceiros.

## 1. Pré-verificação sem alterações

Obtenha o projeto, confira o commit desejado e revise o script antes de executar
com privilégios. Não usar host de produção, VLAN existente ou paths compartilhados.
Comandos de inspeção no Ubuntu:

```sh
cat /etc/os-release
id
uname -m
git status --short
git rev-parse HEAD
df -h /opt /tmp /var/cache/apt
ss -ltn
getent passwd kore-edge
systemctl cat kore-edge.service
```

Ausência de usuário/unidade antes da primeira instalação é normal. O instalador
recusa destinos já existentes ou parciais; não adota dados/configurações silenciosamente.
Exige pelo menos 2 GiB livres em /opt, e o operador também deve dimensionar /tmp,
cache APT e memória para build/testes. Confirmar listener loopback e porta livre;
não há bind padrão. O exemplo com placeholder não é um endereço detectado:

```sh
bash scripts/install.sh --plan --profile media --listen '<LOOPBACK_IP:PORTA_CONFIRMADOS>'
```

Simulação é portátil e não executa apt, systemctl, criação de usuário ou escrita
em destinos operacionais; pode ser revisada no Git Bash/Windows. A aplicação
recusa sistemas diferentes de Ubuntu 24.04 e containers sem systemd ativo.

## 2. Aplicação explícita somente em laboratório

Antes de aplicar, registre o commit, inventário de pacotes, paths inexistentes e
plano de rollback abaixo. Depois de revisar e autorizar esse laboratório:

```sh
sudo bash scripts/install.sh --apply --lab-only --profile media \
  --listen '<LOOPBACK_IP:PORTA_CONFIRMADOS>'
```

O script:

1. Valida sistema, root para instalação, checkout, destinos, porta e espaço; usa
   lock exclusivo para impedir instaladores simultâneos.
2. Detecta pacotes ausentes com dpkg e executa apt update/install somente quando
   necessário, sem recomendações, com espera de lock/retries de rede limitados.
   Não remove pacotes nem força upgrade de versões já instaladas. Hooks APT e
   efeitos transitivos dos pacotes pertencem ao Ubuntu; revisar o host previamente.
3. Exporta o commit com Git para diretório temporário privado. Compila/testa como
   SUDO_USER não root ou nobody, com HOME/cache próprios; nunca executa testes ou
   `go build` como root. Verifica módulos, vet, suíte e detector de corrida. Uma
   falha interrompe antes de publicar binário/configuração/unidade.
4. Instala edge-agent, edge-ops e edge-provision em
   `/opt/kore-edge/releases/<commit>/`; cria symlink `current` para essa versão.
5. Cria/adota somente conta kore-edge sem login, sem grupos adicionais, com home
   `/var/lib/kore-edge`; cria diretórios privados e YAML de laboratório readonly.
6. Instala `deploy/systemd/kore-edge-installed.service`, recarrega unidades e
   verifica a unidade. Não habilita boot e não inicia por padrão.

Pacotes e eventuais arquivos APT não são revertidos se build/teste falhar. Falha
durante a publicação pode deixar instalação parcial; os destinos são preservados
para inspeção, e uma nova execução não os sobrescreve. Não há rollback automático
de pacotes, conta ou dados. Diretório temporário exclusivo é removido na saída
normal; um crash pode deixá-lo para revisão manual.

Configuração inicial: listener explícito, `mode: laboratory`,
`security.mode: local_readonly`; sem catálogo, streams, multicast, HLS, matriz ou
provisionamento habilitados. O perfil media prepara ferramentas, não canais.
Para habilitar escrita, configurar mTLS/RBAC e confianças conforme fases 04/11.
O YAML inicial não contém chave, token nem dados de parceiro/cliente.

## 3. Inicialização e validação

Para solicitar iniciar apenas o novo serviço na primeira instalação, acrescente
`--start` ao comando. O instalador aguarda readiness por uma janela limitada;
se não confirmar, para somente esse novo serviço e preserva os arquivos.
Não executa `enable`. Em repetição idêntica já instalada, é um no-op: verifica
hashes e não reinicia, mesmo com `--start`. Commit/perfil/listener divergentes
exigem o procedimento de atualização da Fase 12.

Se foi instalado sem `--start`, o operador pode iniciar após revisar os arquivos:

```sh
sudo systemctl start kore-edge.service
systemctl status kore-edge.service --no-pager
curl --noproxy '*' --globoff --fail 'http://<LOOPBACK_IP:PORTA_CONFIRMADOS>/healthz'
curl --noproxy '*' --globoff --fail 'http://<LOOPBACK_IP:PORTA_CONFIRMADOS>/readyz'
```

Dashboard no mesmo listener. Para acessar de outro computador, usar túnel SSH com
ports explicitamente revisadas; o agente não deve ser exposto em 0.0.0.0 como
atalho. Health/readiness não comprovam mídia, VPN ou homologação.

Unidade sem capabilities, conta sem login, sistema de arquivos protegido e escrita
restrita a `/var/lib/kore-edge` e `/var/cache/kore-edge`. Memória limitada a 256 MiB,
64 tasks e restart limitado pelo systemd. Não usa Nginx, firewall ou permissões
de administração de rede. Os paths privados de HLS/provisionamento/auditoria devem
ser escolhidos dentro dessas raízes antes de habilitar recursos opcionais.

`/opt/kore-edge/install-manifest` identifica commit/perfil/listener;
`installed.sha256` permite conferir integridade dos três binários, YAML e unidade.
Modificar YAML depois da instalação faz a verificação de repetição divergir:
isso é uma recusa de sobrescrita, não perda de configuração.

## 4. Rollback da primeira instalação

Antes de executar estes comandos, confirmar que a unidade foi criada por esta
instalação e não existia antes, e que o rollback foi autorizado para o laboratório:

```sh
sudo systemctl stop kore-edge.service
sudo systemctl disable kore-edge.service
sudo rm -- /etc/systemd/system/kore-edge.service
sudo systemctl daemon-reload
sudo unlink /opt/kore-edge/current
```

Isso retira o serviço do uso, preservando configurações, releases e dados. Conferir
que o listener fechou com `ss -ltn`. Não apagar automaticamente `/etc/kore-edge`,
`/var/lib/kore-edge`, auditoria, conta ou pacotes: inventariar conteúdo, proprietários
e dependências antes de qualquer remoção. Para atualizar/restaurar versões instaladas,
seguir [Fase 12](modules/12-operations.md), não rerodar este bootstrap sobre dados.

## Testes desta entrega

Testes Go em `tests/installer` validam sintaxe Bash, simulação core/media, IPv4/IPv6,
argumentos inválidos e recusa de aplicação sem flag de laboratório. Nunca executam
aplicação autorizada. A aplicação completa precisa de VM Ubuntu limpa com snapshot,
conta não root para o build e validação/rollback autorizados; não foi executada no
Windows nem em host de produção. O workflow GitHub executa vet, race, regressão,
estresse do provisionamento e build do painel em runner Ubuntu; falhas não são
ignoradas e o resultado de CI não substitui o aceite físico.
