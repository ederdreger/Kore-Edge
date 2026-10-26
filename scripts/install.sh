#!/usr/bin/env bash
# Ubuntu 24.04 laboratory installer. Default: print plan, never change the host.
set -Eeuo pipefail
umask 077

apply=0 lab=0 start=0 profile=media listen=''
source_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
usage() {
  cat <<'EOF'
Kore Edge - instalador Ubuntu Server 24.04 (laboratorio; homologacao NO-GO)
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
    --profile|--listen)
      (($# >= 2)) || die 'Opcao sem valor.'
      if [[ $1 == --profile ]]; then profile=$2; else listen=$2; fi
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

packages=(ca-certificates curl git golang-go build-essential iproute2)
if [[ $profile == media ]]; then packages+=(ffmpeg wireguard-tools); fi
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
available=$(df -Pk /opt | awk 'NR==2 {print $4}')
[[ $available =~ ^[0-9]+$ ]] && ((available >= 2097152)) || die 'Necessarios pelo menos 2 GiB livres em /opt; revisar tambem /tmp e cache APT.'

missing=()
for package in "${packages[@]}"; do
  status=$(dpkg-query -W -f='${Status}' "$package" 2>/dev/null || true)
  [[ $status == 'install ok installed' ]] || missing+=("$package")
done
if ((${#missing[@]})); then
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
systemctl daemon-reload
systemd-analyze verify /etc/systemd/system/kore-edge.service
printf 'revision=%s\nprofile=%s\nlisten=%s\n' "$revision" "$profile" "$listen" >/opt/kore-edge/install-manifest
chmod 0600 /opt/kore-edge/install-manifest
(cd /opt/kore-edge && sha256sum "releases/$revision/edge-agent" "releases/$revision/edge-ops" "releases/$revision/edge-provision" /etc/kore-edge/edge.yaml /etc/systemd/system/kore-edge.service >installed.sha256)
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
fi
printf '%s\n' 'Instalacao concluida. Sem enable no boot. Homologacao permanece NO-GO.'
printf 'Config: /etc/kore-edge/edge.yaml\nPainel local: http://%s/\n' "$listen"
