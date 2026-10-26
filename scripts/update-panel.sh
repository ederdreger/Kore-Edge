#!/usr/bin/env bash
# Reviewed update of an existing installer-managed panel; never installs packages.
set -Eeuo pipefail
umask 077
source_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
source "$source_dir/scripts/gateway-lib.sh"
apply=0
case "${1:---plan}" in --plan|--help) ;; --apply) apply=1 ;; *) printf 'Uso: bash scripts/update-panel.sh [--plan|--apply]\n' >&2; exit 1 ;; esac
(($#<=1)) || exit 1
cat <<'PLAN'
Atualizacao do painel Kore Edge - login, usuarios e instalacao limpa.
Pre-verificacao: checkout limpo, unidades Kore Edge conhecidas, backend loopback,
certificado e hash bcrypt existentes privados; sem sobrescrever usuarios existentes.
Compile/teste primeiro como nobody; sem APT, firewall, NIC, VPN ou servidores externos.
Mudancas: nova release; importar administrador existente somente na primeira vez;
configuracao core limpa (desabilita catalogo, mocks e streams sinteticos);
HTTPS com login por sessao. Dados anteriores ficam preservados fora do painel.
Interrupcao: reinicio somente de kore-edge e kore-edge-gateway apos confirmacao.
Rollback automatico: restaurar YAML, Nginx e symlink anteriores e iniciar os dois
servicos anteriores se a validacao falhar. Nenhuma base de usuarios sera apagada.
Pos-validacao: prontidao local, login acessivel e APIs privadas retornando 401.
O operador deve testar o login e a troca de senha no navegador apos a atualizacao.
PLAN
((apply)) || { printf 'SIMULACAO: nenhuma alteracao executada.\n'; exit 0; }
die(){ printf 'ERRO: %s\n' "$1" >&2; exit 1; }
((EUID==0)) && [[ -t 0 && -t 1 ]] || die 'Execute --apply com sudo em terminal interativo.'
for command in git go runuser systemctl systemd-run curl flock cmp nginx; do command -v "$command" >/dev/null || die 'Dependencia ausente; atualizacao nao instala pacotes.'; done
for path in /opt/kore-edge /opt/kore-edge/releases /etc/kore-edge /etc/kore-edge/gateway /etc/kore-edge/gateway/nginx.conf /etc/kore-edge/edge.yaml /var/lib/kore-edge /var/lib/kore-edge/users /run/kore-edge-installer.lock; do [[ ! -L $path ]] || die 'Destino com symlink inesperado.'; done
exec 9>/run/kore-edge-installer.lock
flock -n 9 || die 'Outra instalacao ou recuperacao esta ativa.'
[[ $(id -u kore-edge) -gt 0 && $(id -u nobody) -gt 0 ]] || die 'Conta de servico/build invalida.'
for unit in kore-edge.service kore-edge-gateway.service; do
 systemctl is-active --quiet "$unit" || die 'Atualizacao exige os dois servicos ativos; use o reparo para instalacao incompleta.'
 [[ $(systemctl show "$unit" -p FragmentPath --value) == "/etc/systemd/system/$unit" ]] || die 'Unidade fora do instalador.'
 cmp -s "$source_dir/deploy/systemd/${unit/kore-edge.service/kore-edge-installed.service}" "/etc/systemd/system/$unit" || die 'Unidade personalizada; revisar atualizacao manualmente.'
done
config=/etc/kore-edge/edge.yaml
gateway=/etc/kore-edge/gateway/nginx.conf
backend=$(sed -n 's/^[[:space:]]*proxy_pass http:\/\/\(127\.0\.0\.1:[0-9]*\);$/\1/p' "$gateway")
host=$(sed -n 's/^[[:space:]]*server_name "\([^"]*\)";$/\1/p' "$gateway")
port=$(sed -n 's/^[[:space:]]*listen 0.0.0.0:\([0-9]*\) ssl;$/\1/p' "$gateway")
valid_public_host "$host" && valid_gateway_port "$port" && [[ $backend =~ ^127\.0\.0\.1:[0-9]+$ ]] || die 'Gateway nao reconhecido.'
ipv6=0; if grep -q 'listen \[::\]:' "$gateway"; then ipv6=1; fi
work=$(mktemp -d /tmp/kore-edge-panel.XXXXXXXX)
chmod 0755 "$work"
trap 'rm -rf -- "$work"' EXIT
# Refuse custom proxy directives rather than dropping security settings silently.
render_gateway "$host" "$port" "$backend" "$ipv6" >"$work/legacy"
render_panel_gateway "$host" "$port" "$backend" "$ipv6" >"$work/panel"
sed '/^[[:space:]]*$/d' "$gateway" >"$work/actual"
if ! cmp -s <(sed '/^[[:space:]]*$/d' "$work/legacy") "$work/actual" && ! cmp -s <(sed '/^[[:space:]]*$/d' "$work/panel") "$work/actual"; then die 'Gateway personalizado; revisar antes de substituir.'; fi
revision=$(git -c safe.directory="$source_dir" -C "$source_dir" rev-parse --verify HEAD)
git -c safe.directory="$source_dir" -C "$source_dir" diff --quiet && git -c safe.directory="$source_dir" -C "$source_dir" diff --cached --quiet || die 'Checkout modificado.'
old_release=$(readlink -f /opt/kore-edge/current)
[[ $old_release == /opt/kore-edge/releases/* && -x $old_release/edge-agent ]] || die 'Release atual fora do instalador.'
printf '%s\n' 'Pre-verificacao somente leitura: systemctl is-active kore-edge.service kore-edge-gateway.service'
 # The Go parser validates the complete YAML before any configuration is changed.
 printf 'Destino detectado: %s\nBackend: %s\n' "$(gateway_url "$host" "$port")" "$backend"
read -r -p 'Confirma estes enderecos e a atualizacao acima? Digite ATUALIZAR: ' answer
[[ $answer == ATUALIZAR ]] || { printf 'Cancelado.\n'; exit 0; }
install -d -o nobody -g "$(id -gn nobody)" -m 0700 "$work/src" "$work/out" "$work/home" "$work/cache" "$work/gopath"
git -c safe.directory="$source_dir" -C "$source_dir" archive "$revision" >"$work/source.tar"
chmod 0644 "$work/source.tar"
runuser -u nobody -- tar -xf "$work/source.tar" -C "$work/src"
runuser -u nobody -- env HOME="$work/home" GOCACHE="$work/cache" GOPATH="$work/gopath" CGO_ENABLED=1 \
 bash -c 'set -euo pipefail; cd "$1"; go mod download; go mod verify; go vet ./...; go test ./... -count=1 -timeout=180s; go test -race ./... -count=1 -timeout=240s; for name in edge-agent edge-ops edge-provision edge-users; do go build -trimpath -o "$2/$name" "./cmd/$name"; done' _ "$work/src" "$work/out"
release="/opt/kore-edge/releases/$revision-panel"
[[ ! -L $release ]] || die 'Release preparada com symlink.'
if [[ -d $release ]]; then
 for name in edge-agent edge-ops edge-provision edge-users; do cmp -s "$work/out/$name" "$release/$name" || die 'Release existente diverge do build.'; done
fi
install -d -o root -g root -m 0755 "$release"
for name in edge-agent edge-ops edge-provision edge-users; do install -o root -g root -m 0755 "$work/out/$name" "$release/$name"; done
origin=$(gateway_url "$host" "$port"); origin=${origin%/}
"$release/edge-users" --action prepare-config --config "$config" --directory /var/lib/kore-edge/users --origin "$origin" --output "$work/edge.yaml"
backup=$(mktemp -d /etc/kore-edge/panel-before.XXXXXXXX)
cp -p "$config" "$backup/edge.yaml"
cp -p "$gateway" "$backup/nginx.conf"
printf '%s\n' "$old_release" >"$backup/release"
printf 'Backup para rollback: %s\n' "$backup"
changed=0
rollback(){
 local status=$?
 if ((changed)); then
  printf 'Validacao falhou; restaurando configuracao e release anteriores.\n' >&2
  systemctl stop kore-edge-gateway.service kore-edge.service || true
  cp -p "$backup/edge.yaml" "$config"; cp -p "$backup/nginx.conf" "$gateway"
  ln -s "$old_release" /opt/kore-edge/.panel-rollback; mv -Tf /opt/kore-edge/.panel-rollback /opt/kore-edge/current
  systemctl start kore-edge.service kore-edge-gateway.service || printf 'Rollback exige revisao manual.\n' >&2
 fi
 rm -rf -- "$work"
 exit "$status"
}
trap rollback EXIT
changed=1
systemctl stop kore-edge-gateway.service kore-edge.service
if [[ ! -f /var/lib/kore-edge/users/users.json ]]; then
 install -d -o kore-edge -g kore-edge -m 0700 /var/lib/kore-edge/users
 runuser -u kore-edge -- "$release/edge-users" --action import-htpasswd --directory /var/lib/kore-edge/users --file /etc/kore-edge/gateway/htpasswd
fi
install -o root -g kore-edge -m 0640 "$work/edge.yaml" "$config"
install -o kore-edge -g kore-edge -m 0600 "$work/panel" "$gateway"
validate_gateway_as_service "$gateway"
[[ ! -e /opt/kore-edge/.panel-current && ! -L /opt/kore-edge/.panel-current ]] || die 'Symlink temporario preexistente.'
ln -s "$release" /opt/kore-edge/.panel-current
mv -Tf /opt/kore-edge/.panel-current /opt/kore-edge/current
systemctl start kore-edge.service
ready=0
for attempt in {1..30}; do if curl --noproxy '*' --globoff --silent --fail --max-time 2 "http://$backend/readyz" >/dev/null; then ready=1; break; fi; sleep 1; done
((ready)) || die 'API nao ficou pronta.'
[[ $(curl --noproxy '*' --globoff -s -o /dev/null -w '%{http_code}' --max-time 3 "http://$backend/api/v1/auth/me") == 401 ]] || die 'API nao bloqueia acesso anonimo.'
systemctl start kore-edge-gateway.service
systemctl is-active --quiet kore-edge-gateway.service || die 'Gateway nao iniciou.'
# Active systemd state does not mean Nginx has finished opening its TLS listener.
if ! check_panel_https "$host" "$port" /etc/kore-edge/gateway/server.crt; then
 die "Validacao HTTPS falhou: $GATEWAY_CHECK_ERROR. Rollback sera executado."
fi
changed=0
printf 'Atualizacao concluida: %s\nUse o mesmo usuario e senha anteriores. Abra uma janela privativa.\n' "$origin"
printf 'Rollback manual (com servicos parados): restaurar arquivos de %s e o symlink indicado em release.\n' "$backup"
