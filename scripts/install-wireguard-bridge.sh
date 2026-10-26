#!/usr/bin/env bash
# One-time, explicitly reviewed installation. Does not apply a tunnel.
set -Eeuo pipefail
umask 077
apply=0 role='' account='' source_state='' binary=''
die(){ printf 'ERRO: %s\n' "$1" >&2; exit 1; }
while (($#)); do
 case "$1" in
 --plan) apply=0 ;;
 --apply) apply=1 ;;
 --role|--user|--application-state|--binary)
  (($#>=2)) || die 'Opcao sem valor.'
  case "$1" in --role) role=$2;; --user) account=$2;; --application-state) source_state=$2;; --binary) binary=$2;; esac;shift ;;
 --help) printf 'Uso: bash scripts/install-wireguard-bridge.sh --role edge|matrix --user CONTA --application-state CAMINHO_PRIVADO --binary BINARIO_LINUX [--plan|--apply]\n';exit 0 ;;
 *) die 'Opcao desconhecida.' ;;
 esac
 shift
done
[[ $role == edge || $role == matrix ]] || die 'Confirme o papel edge ou matrix.'
[[ $account =~ ^[a-z_][a-z0-9_-]{0,31}$ && $source_state =~ ^/[a-zA-Z0-9_./-]+$ && $source_state != *'/../'* && $binary == /* ]] || die 'Confirme conta, arquivo privado absoluto e binario Linux.'
cat <<PLAN
Auxiliar WireGuard restrito ($role).
Pre-verificacao: Linux/systemd, conta nao-root $account, ip/wg/ss instalados,
binario Linux confiavel, arquivo privado regular: $source_state,
ausencia de unidade e socket conflitantes. Sem instalar pacotes.
Destino: /opt/kore-wg-bridge-$role; unidade kore-wg-bridge-$role.service;
socket /run/kore-wg-bridge-$role/control.sock; diario privado /var/lib/kore-wg-bridge-$role.
Mudanca: instalar SOMENTE o auxiliar root com CAP_NET_ADMIN e socket restrito ao
UID/GID confirmado. O processo principal permanece sem privilegios de rede.
Nenhum tunel, NIC, rota, firewall, encaminhamento ou servico existente sera alterado.
Ativacao exige plano revisado, senha e confirmacao no painel, depois desta instalacao.
Rollback da habilitacao: parar/desabilitar apenas kore-wg-bridge-$role.service;
preservar o diario. Parar o auxiliar NAO derruba um tunel que ja tenha sido aplicado.
Para desfazer rede, usar Desfazer no painel antes de parar o auxiliar e conferir
rotas/interfaces com a baseline. Interfaces preexistentes nao sao assumidas pelo auxiliar.
Pos-validacao: unidade ativa; socket root:$account 0660; painel consegue gerar plano;
sem interface/rota nova ate uma confirmacao de ativacao no painel.
PLAN
((apply)) || { printf 'SIMULACAO: nenhuma mudanca executada.\n';exit 0; }
((EUID==0)) && [[ -t 0 && -t 1 ]] || die 'Aplicacao exige sudo e terminal interativo.'
[[ $(uname -s) == Linux ]] || die 'Somente Linux.'
for tool in systemctl systemd-analyze getent install sha256sum readlink ip wg ss flock; do command -v "$tool" >/dev/null || die 'Dependencia ausente. Revise os pacotes antes de instalar.'; done
uid=$(id -u "$account");gid=$(id -g "$account");((uid>0&&gid>0)) || die 'Conta de aplicacao deve ser nao-root.'
[[ -f $source_state && ! -L $source_state && $(stat -c %u "$source_state") == "$uid" && $(stat -c %a "$source_state") == 600 ]] || die 'Arquivo de estado deve ser privado e pertencer a conta confirmada.'
[[ -f $binary && -x $binary ]] || die 'Binario revisado ausente.'
unit="/etc/systemd/system/kore-wg-bridge-$role.service";destination="/opt/kore-wg-bridge-$role"
for path in "$unit" "$destination" "/run/kore-wg-bridge-$role" "/var/lib/kore-wg-bridge-$role"; do [[ ! -e $path && ! -L $path ]] || die 'Auxiliar ou destino preexistente; revisar upgrade/recuperacao manualmente.'; done
[[ ! -L /run/kore-wg-bridge-install.lock ]] || die 'Lock inesperado.'
exec 9>/run/kore-wg-bridge-install.lock;flock -n 9 || die 'Outra habilitacao ativa.'
printf 'Conta confirmada: %s UID=%s GID=%s\nSHA256 do binario: ' "$account" "$uid" "$gid";sha256sum "$binary"
read -r -p 'Confirma o papel, a conta, o arquivo e a instalacao do auxiliar acima? Digite HABILITAR: ' answer
[[ $answer == HABILITAR ]] || { printf 'Cancelado.\n';exit 0; }
install -d -o root -g root -m 0755 "$destination"
install -o root -g root -m 0755 "$binary" "$destination/edge-wg-bridge"
ip_bin=$(readlink -f "$(command -v ip)");wg_bin=$(readlink -f "$(command -v wg)");ss_bin=$(readlink -f "$(command -v ss)")
cat >"$unit" <<UNIT
[Unit]
Description=Kore WireGuard restricted local delegate ($role)
After=network.target
[Service]
Type=simple
User=root
Group=$gid
WorkingDirectory=$destination
ExecStart=$destination/edge-wg-bridge --role $role --app-uid $uid --app-gid $gid --application-state $source_state --state-directory /var/lib/kore-wg-bridge-$role --socket /run/kore-wg-bridge-$role/control.sock --ip-bin $ip_bin --wg-bin $wg_bin --ss-bin $ss_bin
RuntimeDirectory=kore-wg-bridge-$role
RuntimeDirectoryMode=0750
StateDirectory=kore-wg-bridge-$role
StateDirectoryMode=0700
UMask=0077
CapabilityBoundingSet=CAP_NET_ADMIN CAP_DAC_READ_SEARCH CAP_CHOWN
AmbientCapabilities=CAP_NET_ADMIN CAP_DAC_READ_SEARCH CAP_CHOWN
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_UNIX AF_NETLINK AF_INET AF_INET6
ReadWritePaths=/run/kore-wg-bridge-$role /var/lib/kore-wg-bridge-$role
Restart=on-failure
RestartSec=3
TimeoutStopSec=65
MemoryMax=128M
TasksMax=64
[Install]
WantedBy=multi-user.target
UNIT
chmod 0644 "$unit"
systemd-analyze verify "$unit"
systemctl daemon-reload
if ! systemctl enable --now "kore-wg-bridge-$role.service"; then
 systemctl disable --now "kore-wg-bridge-$role.service" || true
 die 'Auxiliar nao iniciou; unidade e arquivos preservados para revisao. Nenhum tunel aplicado.'
fi
systemctl is-active --quiet "kore-wg-bridge-$role.service" || die 'Auxiliar inativo; confira instalacao e permissoes.'
printf 'Auxiliar habilitado. Revise o plano no painel antes de ativar a conexao.\n'
