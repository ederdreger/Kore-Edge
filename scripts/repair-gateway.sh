#!/usr/bin/env bash
# Repair an inactive, partially installed Kore Edge gateway. Default: plan only.
set -Eeuo pipefail
umask 077
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)/gateway-lib.sh"
apply=0 start=0
for argument in "$@"; do
  case "$argument" in
    --apply) apply=1 ;; --start) start=1 ;;
    --plan) apply=0 ;;
    --help) printf '%s\n' 'Uso: sudo bash scripts/repair-gateway.sh --apply [--start]'; exit 0 ;;
    *) printf '%s\n' 'Opcao desconhecida.' >&2; exit 1 ;;
  esac
done
die() { printf 'ERRO: %s\n' "$1" >&2; exit 1; }
cat <<'EOF'
Reparo Kore Edge: diretorios temporarios do Nginx (laboratorio).
Pre-verificacao: unidades Kore Edge instaladas mas inativas, sem processo Nginx,
configuracao reconhecida, sem symlinks nos destinos privados.
Mudancas: backup de nginx.conf; configurar fastcgi/uwsgi/scgi no cache privado;
criar somente os cinco subdiretorios de cache; validar como kore-edge com
CAP_NET_BIND_SERVICE em unidade transitoria (necessario para porta 443).
Senha, certificado, chave, dominio, porta e binarios serao preservados.
Sem APT, sem recompilacao, sem alteracoes de firewall/rede ou sites do Nginx padrao.
Rollback: se nginx -t falhar, restaurar automaticamente nginx.conf do backup.
Com --start: recarregar unidades e iniciar somente core/gateway verificados;
se a prontidao falhar, parar os novos servicos e preservar a configuracao corrigida.
EOF
if ((!apply)); then printf '%s\n' 'SIMULACAO: nenhuma alteracao realizada.'; exit 0; fi
((EUID == 0)) || die 'Execute o reparo com sudo no laboratorio.'
for command in nginx runuser getent systemctl systemd-analyze systemd-run pgrep flock curl openssl ss; do
  command -v "$command" >/dev/null || die 'Ferramenta necessaria ao reparo ausente.'
done
for path in /etc/kore-edge /etc/kore-edge/gateway /var/cache/kore-edge /var/cache/kore-edge/gateway /run/kore-edge-installer.lock; do
  [[ ! -L $path ]] || die 'Destino com symlink; reparo recusado.'
done
exec 9>/run/kore-edge-installer.lock
flock -n 9 || die 'Outro instalador/reparo esta ativo.'
[[ ! -e /opt/kore-edge/install-manifest && ! -L /opt/kore-edge/install-manifest ]] || die 'Instalacao completa registrada; este reparo e somente para publicacao interrompida.'
for unit in kore-edge.service kore-edge-gateway.service; do
  [[ -f /etc/systemd/system/$unit && ! -L /etc/systemd/system/$unit ]] || die 'Unidade propria ausente ou inesperada.'
  state=$(systemctl is-active "$unit" 2>/dev/null || true)
  [[ $state == inactive || $state == failed || $state == unknown ]] || die 'Servico ativo/em transicao; este reparo exige unidades inativas.'
done
if pgrep -x nginx >/dev/null; then die 'Nginx em execucao; reparo recusado.'; fi
configuration=/etc/kore-edge/gateway/nginx.conf
for file in "$configuration" /etc/kore-edge/edge.yaml /etc/kore-edge/gateway/server.crt /etc/kore-edge/gateway/server.key /etc/kore-edge/gateway/htpasswd; do
  [[ -f $file && ! -L $file ]] || die 'Arquivo instalado ausente ou inesperado.'
done
grep -Fqx '    auth_basic "Kore Edge";' "$configuration" || die 'Configuracao nao reconhecida.'
grep -Fqx '      limit_except GET HEAD { deny all; }' "$configuration" || die 'Protecao de leitura nao reconhecida.'
grep -Fqx '  proxy_temp_path /var/cache/kore-edge/gateway/proxy;' "$configuration" || die 'Cache nao reconhecido.'
public_host=$(sed -n 's/^    server_name "\([^"]*\)";$/\1/p' "$configuration")
public_port=$(sed -n 's/^    listen 0\.0\.0\.0:\([0-9]*\) ssl;$/\1/p' "$configuration")
backend=$(sed -n 's/^      proxy_pass http:\/\/\(127\.0\.0\.1:[0-9]*\);$/\1/p' "$configuration")
valid_public_host "$public_host" && valid_gateway_port "$public_port" || die 'Dominio/porta instalados invalidos.'
[[ $backend =~ ^127\.0\.0\.1:([1-9][0-9]{0,4})$ ]] && valid_gateway_port "${BASH_REMATCH[1]}" || die 'Backend nao reconhecido.'
grep -Fqx "  listen: \"$backend\"" /etc/kore-edge/edge.yaml || die 'Backend diverge do YAML instalado.'
grep -Fqx 'ExecStart=/opt/kore-edge/current/edge-agent -config /etc/kore-edge/edge.yaml' /etc/systemd/system/kore-edge.service || die 'Unidade core nao reconhecida.'
grep -Fqx 'ExecStart=/usr/sbin/nginx -c /etc/kore-edge/gateway/nginx.conf -g "daemon off;"' /etc/systemd/system/kore-edge-gateway.service || die 'Unidade gateway nao reconhecida.'
[[ $(id -u kore-edge) -gt 0 ]] || die 'Conta do servico invalida.'
if ((start)); then
  for port in "${backend##*:}" "$public_port"; do
    if ss -H -ltn "sport = :$port" | grep -q .; then die 'Porta escolhida ocupada; inicio recusado sem tocar no processo existente.'; fi
  done
fi

backup=$(mktemp /etc/kore-edge/gateway/nginx.conf.before-temp.XXXXXXXX)
cp --preserve=mode,ownership "$configuration" "$backup"
candidate=$(mktemp /etc/kore-edge/gateway/.nginx-repair.XXXXXXXX)
validated=0 changed=0
cleanup() {
  if ((changed && !validated)); then cp --preserve=mode,ownership "$backup" "$configuration"; fi
  [[ ! -e $candidate ]] || rm -- "$candidate"
}
trap cleanup EXIT
repair_gateway_temp_paths "$configuration" >"$candidate"
chown kore-edge:kore-edge "$candidate"; chmod 0600 "$candidate"
for temp in client proxy fastcgi uwsgi scgi; do
  [[ ! -L /var/cache/kore-edge/gateway/$temp ]] || die 'Subdiretorio de cache com symlink.'
  install -d -o kore-edge -g kore-edge -m 0700 "/var/cache/kore-edge/gateway/$temp"
done
mv -T -- "$candidate" "$configuration"; changed=1
validate_gateway_as_service "$configuration"
validated=1
systemctl daemon-reload
systemd-analyze verify /etc/systemd/system/kore-edge.service /etc/systemd/system/kore-edge-gateway.service
if ((start)); then
  systemctl reset-failed kore-edge.service kore-edge-gateway.service
  systemctl start kore-edge.service || { systemctl stop kore-edge.service; die 'Core nao iniciou; configuracao preservada.'; }
  ready=0
  for ((attempt=0;attempt<20;attempt++)); do
    if curl --noproxy '*' --fail --silent --max-time 2 "http://$backend/readyz" >/dev/null; then ready=1; break; fi
    sleep 1
  done
  if ((!ready)); then systemctl stop kore-edge.service; die 'Core nao ficou pronto.'; fi
  systemctl start kore-edge-gateway.service || { systemctl stop kore-edge-gateway.service kore-edge.service; die 'Gateway nao iniciou; configuracao corrigida preservada.'; }
  systemctl is-active --quiet kore-edge-gateway.service || { systemctl stop kore-edge-gateway.service kore-edge.service; die 'Gateway indisponivel.'; }
fi
printf 'Reparo validado. Backup: %s\nPainel: %s\n' "$backup" "$(gateway_url "$public_host" "$public_port")"
printf 'Endereco: %s\n' "$(gateway_host_scope "$public_host")"
printf '%s\n' 'IP privado: acesso pela LAN/VPN. Internet depende de NAT/DNS e firewall do provedor.'
printf '%s\n' 'Usuario, senha, certificado e binarios preservados. Sem enable no boot.'
openssl x509 -noout -fingerprint -sha256 -in /etc/kore-edge/gateway/server.crt
