#!/usr/bin/env bash
# Ubuntu 24.04 laboratory installer. Default: print plan, never change the host.
set -Eeuo pipefail
umask 077

apply=0 lab=0 start=0 profile=media listen=''
resume_packages=0
public_host='' public_port='' gateway_user=admin
source_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
source "$source_dir/scripts/gateway-lib.sh"
if (($# == 0)) && [[ -t 0 && -t 1 ]]; then
  exec bash "$source_dir/scripts/setup.sh"
fi
usage() {
  cat <<'EOF'
Kore Edge - instalador Ubuntu Server 24.04 (laboratorio; homologacao NO-GO)
  bash scripts/setup.sh   # assistente simples com perguntas por etapas
  bash scripts/install.sh --listen IP:PORTA [--profile core|media] [--plan]
  sudo bash scripts/install.sh --listen IP:PORTA --apply --lab-only [--start]

--plan       Padrao: apenas mostrar pre-verificacao, mudancas e rollback.
--apply      Instalar pacotes, compilar/testar como usuario sem privilegios,
             instalar configuracao somente leitura e unidade systemd.
--lab-only   Confirmar explicitamente que o host e de laboratorio, nao producao.
--start      Iniciar somente o novo servico; nao habilitar no boot.
--profile    media (padrao): FFmpeg e wireguard-tools; core: somente API/dashboard.
--listen     IP literal de loopback e porta explicitos, por exemplo os valores
             revisados pelo operador. Nao abre portas nem configura VPN/NIC.
--public-host Dominio ou IP da VPS para gateway HTTPS com senha (opcional).
--public-port Porta HTTPS explicita; exige --public-host e backend 127.0.0.1.
--gateway-user Usuario do painel [admin]. Senha somente no terminal, nunca argv.
--resume-packages Retomar pacotes de tentativa interrompida; Nginx deve estar mascarado e inativo.

Nao ha instalacao por curl|sudo bash. Revise o checkout e o plano antes de aplicar.
EOF
}
die() { printf 'ERRO: %s\n' "$1" >&2; exit 1; }
while (($#)); do
  case "$1" in
    --help|-h) usage; exit 0 ;;
    --plan) apply=0 ;;
    --apply) apply=1 ;;
    --lab-only) lab=1 ;;
    --start) start=1 ;;
    --resume-packages) resume_packages=1 ;;
    --profile|--listen|--public-host|--public-port|--gateway-user)
      (($# >= 2)) || die 'Opcao sem valor.'
      case "$1" in
        --profile) profile=$2 ;; --listen) listen=$2 ;;
        --public-host) public_host=${2,,} ;; --public-port) public_port=$2 ;;
        --gateway-user) gateway_user=$2 ;;
      esac
      shift ;;
    *) die 'Opcao desconhecida; consulte --help.' ;;
  esac
  shift
done
[[ $profile == core || $profile == media ]] || die 'Perfil invalido.'
[[ -n $listen ]] || die '--listen e obrigatorio; nao existe porta assumida.'
port=''
if [[ $listen =~ ^127\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3}):([0-9]{1,5})$ ]]; then
  for octet in "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}"; do
    ((10#$octet <= 255)) || die 'IPv4 invalido.'
    [[ $octet == 0 || $octet != 0* ]] || die 'IPv4 com zeros ambiguos.'
  done
  port=${BASH_REMATCH[4]}
elif [[ $listen =~ ^\[::1\]:([0-9]{1,5})$ ]]; then port=${BASH_REMATCH[1]}
else die 'Use IP literal de loopback IPv4 127.x.x.x ou [::1] e porta.'; fi
((10#$port >= 1 && 10#$port <= 65535)) || die 'Porta fora de 1..65535.'
[[ $port != 0* ]] || die 'Porta com zero inicial.'
if [[ -n $public_host ]]; then
  valid_public_host "$public_host" && valid_gateway_port "$public_port" || die 'Dominio/IP da VPS ou porta HTTPS invalidos.'
  [[ $gateway_user =~ ^[a-zA-Z][a-zA-Z0-9_-]{0,31}$ ]] || die 'Usuario do painel invalido.'
  [[ $listen == "127.0.0.1:$port" && $port != "$public_port" ]] || die 'Gateway exige backend IPv4 loopback e portas distintas.'
elif [[ -n $public_port || $gateway_user != admin ]]; then die '--public-port/--gateway-user exigem --public-host.'; fi
if ((resume_packages)) && [[ -z $public_host ]]; then die 'Retomada de pacotes Nginx exige --public-host.'; fi

packages=(ca-certificates curl git golang-go build-essential iproute2)
if [[ $profile == media ]]; then packages+=(ffmpeg wireguard-tools); fi
if [[ -n $public_host ]]; then packages+=(nginx openssl apache2-utils); fi
printf '%s\n' 'Kore Edge: plano de laboratorio; implantacao permanece NO-GO.'
printf 'Perfil: %s\nListener: %s\nAPT (somente pacotes ausentes): %s\n' "$profile" "$listen" "${packages[*]}"
cat <<'EOF'
Pre-verificacao: Ubuntu 24.04, root somente na instalacao, systemd, espaco,
porta livre, checkout Git limpo, ausencia de instalacao/servico conflitante.
Build: snapshot do commit Git, go mod verify, vet, testes e race detector;
executados sem root. Dashboard compilada incorporada; Node/npm nao sao runtime.
Destinos: /opt/kore-edge, /etc/kore-edge/edge.yaml,
/var/lib/kore-edge, /var/cache/kore-edge, /etc/systemd/system/kore-edge.service.
Conta: kore-edge sem login; servico sem capabilities, somente leitura em loopback.
Nao altera firewall, NIC, rotas, tunel WireGuard, catalogo ou hosts externos.
Rollback: parar apenas kore-edge se iniciado, retirar unidade/symlink novos;
preservar config/dados e revisar remocao de pacotes/conta separadamente.
Pacotes APT nao sao revertidos automaticamente; hooks de pacotes pertencem ao Ubuntu.
EOF
if [[ -n $public_host ]]; then
  printf 'Painel da VPS: %s\n' "$(gateway_url "$public_host" "$public_port")"
  printf '%s\n' 'Entrada publica HTTPS com senha, somente GET/HEAD; API continua privada.'
  printf '%s\n' 'Certificado local autoassinado (aviso no navegador); sem ACME automatico.'
  printf '%s\n' 'Gateway escuta interfaces publicas na porta escolhida. Firewall/DNS nao serao alterados.'
  printf '%s\n' 'Nginx preexistente nao e adotado. A unidade nginx padrao sera mascarada antes do APT.'
  if ((resume_packages)); then printf '%s\n' 'Retomada solicitada: reutilizar pacotes, sem modificar sites existentes. Nginx deve estar mascarado/inativo.'; fi
  printf '%s\n' 'Rollback adicional: parar/remover apenas kore-edge-gateway e revisar a mascara nginx criada.'
fi
if (( ! apply )); then
  printf '%s\n' 'SIMULACAO: nenhum comando operacional executado.'
  exit 0
fi
((lab)) || die 'Aplicacao exige --lab-only; producao nao esta homologada.'
((EUID == 0)) || die 'Execute --apply como root somente no host de laboratorio.'
[[ -f /etc/os-release ]] || die 'Sistema nao identificado.'
# os-release is a trusted OS-owned file, not an installer argument.
. /etc/os-release
[[ ${ID:-} == ubuntu && ${VERSION_ID:-} == 24.04 ]] || die 'Suportado somente Ubuntu 24.04.'
[[ -d /run/systemd/system ]] || die 'Requer host Ubuntu com systemd ativo, nao container sem systemd.'
for command in apt-get dpkg-query flock getent runuser useradd install tar git ss sha256sum df systemctl systemd-analyze; do
  command -v "$command" >/dev/null || die 'Ferramenta de pre-verificacao ausente; consulte docs/INSTALL.md.'
done
for path in /opt/kore-edge /etc/kore-edge /var/lib/kore-edge /var/cache/kore-edge /etc/systemd/system/kore-edge.service /run/kore-edge-installer.lock; do
  [[ ! -L $path ]] || die 'Destino operacional com symlink; revisar manualmente.'
done
exec 9>/run/kore-edge-installer.lock
flock -n 9 || die 'Outro instalador esta ativo.'
revision=$(git -c safe.directory="$source_dir" -C "$source_dir" rev-parse --verify HEAD)
git -c safe.directory="$source_dir" -C "$source_dir" diff --quiet || die 'Checkout modificado; revise e commit antes de instalar.'
git -c safe.directory="$source_dir" -C "$source_dir" diff --cached --quiet || die 'Index modificado; revise e commit antes de instalar.'
[[ -f $source_dir/internal/webui/dist/index.html ]] || die 'Dashboard compilada ausente.'
if [[ -f /opt/kore-edge/install-manifest ]]; then
  [[ ! -L /opt/kore-edge/install-manifest ]] || die 'Manifesto com symlink.'
  expected=$(printf 'revision=%s\nprofile=%s\nlisten=%s' "$revision" "$profile" "$listen")
  if [[ -n $public_host ]]; then expected=$(printf '%s\npublic_host=%s\npublic_port=%s\ngateway_user=%s' "$expected" "$public_host" "$public_port" "$gateway_user"); fi
  actual=$(cat /opt/kore-edge/install-manifest)
  [[ $actual == "$expected" ]] || die 'Instalacao existente difere; usar procedimento de upgrade/rollback da Fase 12.'
  (cd /opt/kore-edge && sha256sum --check --status installed.sha256) || die 'Integridade instalada divergente.'
  printf '%s\n' 'Ja instalado com este commit/perfil/listener. Nenhuma alteracao ou restart.'
  exit 0
fi
for path in /opt/kore-edge /etc/kore-edge /var/lib/kore-edge /var/cache/kore-edge /etc/systemd/system/kore-edge.service; do
  [[ ! -e $path && ! -L $path ]] || die 'Instalacao existente ou incompleta; nao sera sobrescrita.'
done
if systemctl cat kore-edge.service >/dev/null 2>&1; then die 'Unidade kore-edge existente; revisar antes de instalar.'; fi
if ss -H -ltn "sport = :$port" | grep -q .; then die 'Porta ocupada; escolha e confirme outro listener.'; fi
if [[ -n $public_host ]]; then
  [[ -t 0 ]] || die 'Senha do gateway exige terminal interativo.'
  [[ ! -e /etc/systemd/system/kore-edge-gateway.service && ! -L /etc/systemd/system/kore-edge-gateway.service ]] || die 'Gateway preexistente; nao sera sobrescrito.'
  if ((resume_packages)); then
    command -v pgrep >/dev/null || die 'pgrep ausente; nao e possivel verificar processos Nginx na retomada.'
    # Reuse the package binary only. The default sites/config remain untouched.
    [[ -L /etc/systemd/system/nginx.service && $(readlink /etc/systemd/system/nginx.service) == /dev/null ]] || die 'Retomada exige a mascara nginx criada na tentativa anterior.'
    [[ $(systemctl is-enabled nginx.service 2>/dev/null || true) == masked ]] || die 'Estado da mascara nginx divergente.'
    if systemctl is-active --quiet nginx.service || pgrep -x nginx >/dev/null; then die 'Nginx em execucao; retomada recusada.'; fi
  else
    [[ ! -e /etc/nginx && ! -L /etc/systemd/system/nginx.service ]] || die 'Nginx preexistente; se uma tentativa falhou apos APT, use a retomada guiada.'
    if command -v nginx >/dev/null 2>&1 || systemctl cat nginx.service >/dev/null 2>&1; then die 'Nginx preexistente; revisar configuracao manualmente.'; fi
  fi
  if ss -H -ltn "sport = :$public_port" | grep -q .; then die 'Porta HTTPS ocupada.'; fi
  # These secrets stay shell-local until written to the private bcrypt file.
  while :; do
    read -r -s -p 'Senha do painel (12 a 72 caracteres): ' gateway_password || die 'Entrada encerrada.'; printf '\n'
    read -r -s -p 'Repita a senha: ' gateway_confirmation || die 'Entrada encerrada.'; printf '\n'
    password_bytes=$(printf '%s' "$gateway_password" | LC_ALL=C wc -c)
    if [[ ${#gateway_password} -ge 12 && $password_bytes -le 72 && $gateway_password == "$gateway_confirmation" && $gateway_password != *:* ]]; then break; fi
    printf '%s\n' 'Senhas diferentes ou tamanho invalido; use 12 a 72 caracteres, sem dois-pontos.'
  done
  unset gateway_confirmation
fi
available=$(df -Pk /opt | awk 'NR==2 {print $4}')
[[ $available =~ ^[0-9]+$ ]] && ((available >= 2097152)) || die 'Necessarios pelo menos 2 GiB livres em /opt; revisar tambem /tmp e cache APT.'

missing=()
for package in "${packages[@]}"; do
  status=$(dpkg-query -W -f='${Status}' "$package" 2>/dev/null || true)
  [[ $status == 'install ok installed' ]] || missing+=("$package")
done
if ((${#missing[@]})); then
  if [[ -n $public_host ]]; then systemctl mask nginx.service; fi
  export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l
  apt-get -o DPkg::Lock::Timeout=60 -o Acquire::Retries=2 -o Acquire::http::Timeout=30 -o Acquire::https::Timeout=30 update
  apt-get -o DPkg::Lock::Timeout=60 -o Acquire::Retries=2 -o Acquire::http::Timeout=30 -o Acquire::https::Timeout=30 install -y --no-install-recommends "${missing[@]}"
fi

work=$(mktemp -d /tmp/kore-edge-install.XXXXXXXX)
cleanup() {
  # Only remove the unique directory allocated by this invocation, never a target.
  if [[ ${work:-} == /tmp/kore-edge-install.* && -d $work && ! -L $work ]]; then rm -rf -- "$work"; fi
}
trap cleanup EXIT
build_user=nobody
if [[ -n ${SUDO_USER:-} && $SUDO_USER != root ]]; then build_user=$SUDO_USER; fi
build_uid=$(id -u "$build_user")
((build_uid > 0)) || die 'Compilacao como root recusada.'
git -c safe.directory="$source_dir" -C "$source_dir" archive --format=tar "$revision" >"$work/source.tar"
mkdir "$work/src" "$work/home" "$work/out"
chown -R "$build_user" "$work"
runuser -u "$build_user" -- tar -xf "$work/source.tar" -C "$work/src"
runuser -u "$build_user" -- env HOME="$work/home" GOCACHE="$work/cache" GOPATH="$work/gopath" CGO_ENABLED=1 \
  bash -c 'set -euo pipefail; cd "$1"; go mod download; go mod verify; go vet ./...; go test ./... -count=1 -timeout=120s; go test -race ./... -count=1 -timeout=180s; for name in edge-agent edge-ops edge-provision; do go build -trimpath -o "$2/$name" "./cmd/$name"; done' _ "$work/src" "$work/out"

if getent passwd kore-edge >/dev/null; then
  IFS=: read -r _ _ service_uid _ _ service_home service_shell < <(getent passwd kore-edge)
  ((service_uid > 0)) && [[ $service_home == /var/lib/kore-edge && $service_shell == /usr/sbin/nologin ]] || die 'Conta kore-edge existente nao corresponde ao servico sem login.'
  getent group kore-edge >/dev/null || die 'Grupo de servico ausente.'
  service_gid=$(getent group kore-edge | cut -d: -f3)
  [[ $(id -G kore-edge) == "$service_gid" ]] || die 'Conta existente possui grupos adicionais ou primario inesperado.'
else
  useradd --system --user-group --home-dir /var/lib/kore-edge --no-create-home --shell /usr/sbin/nologin kore-edge
fi
install -d -o root -g root -m 0755 /opt/kore-edge /opt/kore-edge/releases
install -d -o root -g kore-edge -m 0750 /etc/kore-edge
install -d -o kore-edge -g kore-edge -m 0700 /var/lib/kore-edge /var/cache/kore-edge
release="/opt/kore-edge/releases/$revision"
install -d -o root -g root -m 0755 "$release"
for name in edge-agent edge-ops edge-provision; do install -o root -g root -m 0755 "$work/out/$name" "$release/$name"; done
install -o root -g kore-edge -m 0640 /dev/null /etc/kore-edge/edge.yaml
cat >/etc/kore-edge/edge.yaml <<EOF
version: 1
mode: laboratory
security:
  mode: local_readonly
http:
  listen: "$listen"
EOF
ln -s "releases/$revision" /opt/kore-edge/current
git -c safe.directory="$source_dir" -C "$source_dir" show "$revision:deploy/systemd/kore-edge-installed.service" >"$release/kore-edge.service"
install -o root -g root -m 0644 "$release/kore-edge.service" /etc/systemd/system/kore-edge.service
if [[ -n $public_host ]]; then
  install -d -o kore-edge -g kore-edge -m 0700 /etc/kore-edge/gateway /var/cache/kore-edge/gateway
  for temp in client proxy fastcgi uwsgi scgi; do
    install -d -o kore-edge -g kore-edge -m 0700 "/var/cache/kore-edge/gateway/$temp"
  done
  san="DNS:$public_host"
  if [[ $public_host == *:* || $public_host =~ ^[0-9.]+$ ]]; then san="IP:$public_host"; fi
  openssl req -x509 -newkey rsa:3072 -nodes -sha256 -days 365 -subj '/CN=Kore Edge laboratory' \
    -addext "subjectAltName=$san" -keyout /etc/kore-edge/gateway/server.key -out /etc/kore-edge/gateway/server.crt 2>/dev/null
  printf '%s\n' "$gateway_password" | htpasswd -iBc -C 10 /etc/kore-edge/gateway/htpasswd "$gateway_user" >/dev/null 2>&1
  unset gateway_password
  ipv6=0; if [[ -r /proc/net/if_inet6 ]] && grep -q . /proc/net/if_inet6; then ipv6=1; fi
  if [[ $public_host == *:* && $ipv6 == 0 ]]; then die 'IPv6 escolhido, mas indisponivel no host.'; fi
  render_gateway "$public_host" "$public_port" "$listen" "$ipv6" >/etc/kore-edge/gateway/nginx.conf
  chown kore-edge:kore-edge /etc/kore-edge/gateway/{server.key,server.crt,htpasswd,nginx.conf}
  chmod 0600 /etc/kore-edge/gateway/{server.key,server.crt,htpasswd,nginx.conf}
  git -c safe.directory="$source_dir" -C "$source_dir" show "$revision:deploy/systemd/kore-edge-gateway.service" >"$release/kore-edge-gateway.service"
  install -o root -g root -m 0644 "$release/kore-edge-gateway.service" /etc/systemd/system/kore-edge-gateway.service
  runuser -u kore-edge -- nginx -t -q -c /etc/kore-edge/gateway/nginx.conf
fi
systemctl daemon-reload
systemd-analyze verify /etc/systemd/system/kore-edge.service
if [[ -n $public_host ]]; then systemd-analyze verify /etc/systemd/system/kore-edge-gateway.service; fi
printf 'revision=%s\nprofile=%s\nlisten=%s\n' "$revision" "$profile" "$listen" >/opt/kore-edge/install-manifest
if [[ -n $public_host ]]; then printf 'public_host=%s\npublic_port=%s\ngateway_user=%s\n' "$public_host" "$public_port" "$gateway_user" >>/opt/kore-edge/install-manifest; fi
chmod 0600 /opt/kore-edge/install-manifest
(cd /opt/kore-edge && sha256sum "releases/$revision/edge-agent" "releases/$revision/edge-ops" "releases/$revision/edge-provision" /etc/kore-edge/edge.yaml /etc/systemd/system/kore-edge.service >installed.sha256)
if [[ -n $public_host ]]; then sha256sum /etc/kore-edge/gateway/nginx.conf /etc/systemd/system/kore-edge-gateway.service >>/opt/kore-edge/installed.sha256; fi
if ((start)); then
  systemctl start kore-edge.service
  ready=0
  for ((attempt=0;attempt<20;attempt++)); do
    if curl --noproxy '*' --globoff --fail --silent --max-time 2 "http://$listen/readyz" >/dev/null; then ready=1; break; fi
    sleep 1
  done
  if ((!ready)); then
    systemctl stop kore-edge.service
    die 'Prontidao nao confirmada; somente o novo servico foi parado. Config/dados preservados.'
  fi
  if [[ -n $public_host ]]; then
    systemctl start kore-edge-gateway.service || { systemctl stop kore-edge.service; die 'Gateway nao iniciou; backend novo foi parado.'; }
    systemctl is-active --quiet kore-edge-gateway.service || { systemctl stop kore-edge-gateway.service kore-edge.service; die 'Gateway indisponivel; novos servicos parados.'; }
  fi
fi
printf '%s\n' 'Instalacao concluida. Sem enable no boot. Homologacao permanece NO-GO.'
printf 'Config: /etc/kore-edge/edge.yaml\nPainel local: http://%s/\n' "$listen"
if [[ -n $public_host ]]; then
  printf 'Painel da VPS: %s\n' "$(gateway_url "$public_host" "$public_port")"
  printf '%s\n' 'O navegador pedira usuario/senha e avisara sobre o certificado local. Confira a impressao digital:'
  openssl x509 -noout -fingerprint -sha256 -in /etc/kore-edge/gateway/server.crt
fi
