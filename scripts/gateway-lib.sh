#!/usr/bin/env bash
# Pure validation/rendering helpers. Sourcing this file performs no host changes.
valid_public_host() {
  local host=$1 label octet part rest count=0 compressed=0
  [[ -n $host && ${#host} -le 253 ]] || return 1
  if [[ $host == *:* ]]; then
    [[ $host =~ ^[a-fA-F0-9:]+$ && $host != *:::* && $host != :: && $host != ::1 ]] || return 1
    if [[ $host == *::* ]]; then
      compressed=1; rest=${host#*::}; [[ $rest != *::* ]] || return 1
      host=${host/::/:}
    else
      [[ $host != :* && $host != *: ]] || return 1
    fi
    host=${host#:}; host=${host%:}
    local -a parts=(); IFS=: read -r -a parts <<<"$host"
    for part in "${parts[@]}"; do [[ $part =~ ^[a-fA-F0-9]{1,4}$ ]] || return 1; count=$((count+1)); done
    if ((compressed)); then ((count < 8)); else ((count == 8)); fi
    return
  fi
  if [[ $host =~ ^[0-9.]+$ ]]; then
    [[ $host =~ ^([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})$ ]] || return 1
    local first=${BASH_REMATCH[1]}
    for octet in "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}" "${BASH_REMATCH[4]}"; do
      ((10#$octet <= 255)) || return 1
      [[ $octet == 0 || $octet != 0* ]] || return 1
    done
    ((10#$first > 0 && 10#$first != 127 && 10#$first < 224)); return
  fi
  [[ $host == *.* && $host != *..* && $host != .* && $host != *. ]] || return 1
  local -a labels=(); IFS=. read -r -a labels <<<"$host"
  for label in "${labels[@]}"; do
    [[ ${#label} -le 63 && $label =~ ^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$ ]] || return 1
  done
}
valid_gateway_port() { [[ $1 =~ ^[1-9][0-9]{0,4}$ ]] && ((10#$1 <= 65535)); }
gateway_host_scope() {
  local host=${1,,} second
  if [[ $host == *:* ]]; then
    case "$host" in fc*:*|fd*:*) printf 'IP privado IPv6 (ULA)' ;; *) printf 'IP IPv6' ;; esac
  elif [[ $host =~ ^[0-9.]+$ ]]; then
    case "$host" in
      10.*|192.168.*) printf 'IP privado IPv4' ;;
      172.*) second=${host#172.}; second=${second%%.*}; if ((10#$second>=16 && 10#$second<=31)); then printf 'IP privado IPv4'; else printf 'IP IPv4 nao privado'; fi ;;
      100.*) second=${host#100.}; second=${second%%.*}; if ((10#$second>=64 && 10#$second<=127)); then printf 'IP compartilhado CGNAT'; else printf 'IP IPv4 nao privado'; fi ;;
      *) printf 'IP IPv4 nao privado' ;;
    esac
  else printf 'Dominio (alcance depende do DNS)'; fi
}

# The validator needs the same unprivileged user AND bind capability as the
# service, since nginx -t can check sockets on privileged HTTPS ports.
validate_gateway_as_service() {
  local configuration=$1
  systemd-run --quiet --wait --pipe --collect --unit="kore-edge-gateway-check-$$" \
    --property=Type=oneshot --property=User=kore-edge --property=Group=kore-edge \
    --property=AmbientCapabilities=CAP_NET_BIND_SERVICE \
    --property=CapabilityBoundingSet=CAP_NET_BIND_SERVICE \
    --property=NoNewPrivileges=yes --property=ProtectSystem=strict \
    --property=ProtectHome=yes --property=PrivateTmp=yes \
    --property=ReadWritePaths=/var/cache/kore-edge/gateway \
    --property=TimeoutStartSec=20s \
    /usr/sbin/nginx -t -q -c "$configuration"
}

# Fresh units can exist on disk without a loaded failed-state entry. Reset only
# failures that actually exist; do not make first startup depend on reset-failed.
reset_kore_failed_units() {
  local unit
  for unit in kore-edge.service kore-edge-gateway.service; do
    if systemctl is-failed --quiet "$unit"; then
      systemctl reset-failed "$unit" || return 1
    fi
  done
}
gateway_url() {
  local host=$1 port=$2
  if [[ $host == *:* ]]; then host="[$host]"; fi
  printf 'https://%s:%s/' "$host" "$port"
}
render_gateway() {
  local host=$1 port=$2 backend=$3 ipv6=${4:-0}
  valid_public_host "$host" && valid_gateway_port "$port" || return 1
  [[ $backend =~ ^127\.0\.0\.1:([1-9][0-9]{0,4})$ ]] && valid_gateway_port "${BASH_REMATCH[1]}" || return 1
  cat <<EOF
worker_processes 1;
pid /var/cache/kore-edge/gateway/nginx.pid;
error_log /dev/null emerg;
events { worker_connections 128; }
http {
  access_log off;
  server_tokens off;
  client_body_temp_path /var/cache/kore-edge/gateway/client;
  proxy_temp_path /var/cache/kore-edge/gateway/proxy;
  fastcgi_temp_path /var/cache/kore-edge/gateway/fastcgi;
  uwsgi_temp_path /var/cache/kore-edge/gateway/uwsgi;
  scgi_temp_path /var/cache/kore-edge/gateway/scgi;
  limit_req_zone \$binary_remote_addr zone=login:1m rate=10r/s;
  server {
    listen 0.0.0.0:$port ssl;
EOF
  if [[ $ipv6 == 1 ]]; then printf '    listen [::]:%s ssl ipv6only=on;\n' "$port"; fi
  cat <<EOF
    server_name "$host";
    ssl_certificate /etc/kore-edge/gateway/server.crt;
    ssl_certificate_key /etc/kore-edge/gateway/server.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_tickets off;
    auth_basic "Kore Edge";
    auth_basic_user_file /etc/kore-edge/gateway/htpasswd;
    client_max_body_size 1k;
    client_header_timeout 10s;
    client_body_timeout 10s;
    keepalive_timeout 15s;
    add_header X-Content-Type-Options nosniff always;
    add_header X-Frame-Options DENY always;
    location / {
      limit_req zone=login burst=30 nodelay;
      limit_except GET HEAD { deny all; }
      proxy_connect_timeout 3s;
      proxy_read_timeout 10s;
      proxy_pass http://$backend;
      proxy_set_header Host "$backend";
      proxy_set_header Origin "";
      proxy_set_header Authorization "";
      proxy_set_header X-Forwarded-For "";
      proxy_set_header X-Forwarded-Client-Cert "";
    }
  }
}

EOF
}


# Patch only the three omitted directives, retaining certificates, auth and ports.
repair_gateway_temp_paths() {
  awk '
    /^[[:space:]]*(fastcgi|uwsgi|scgi)_temp_path[[:space:]]/ { next }
    { print }
    /^[[:space:]]*proxy_temp_path \/var\/cache\/kore-edge\/gateway\/proxy;$/ {
      print "  fastcgi_temp_path /var/cache/kore-edge/gateway/fastcgi;"
      print "  uwsgi_temp_path /var/cache/kore-edge/gateway/uwsgi;"
      print "  scgi_temp_path /var/cache/kore-edge/gateway/scgi;"
    }
  ' "$1"
}
# Session-based panel gateway. Retains TLS/cache isolation from the legacy renderer.
render_legacy_panel_gateway() {
  render_gateway "$@" | sed \
    -e '/^[[:space:]]*auth_basic /d' \
    -e '/^[[:space:]]*auth_basic_user_file /d' \
    -e '/limit_except GET HEAD/d' \
    -e '/proxy_set_header Origin /d' \
    -e 's/client_max_body_size 1k;/client_max_body_size 16k;/'
}

# Only capability-protected embed documents may be framed. The dashboard keeps DENY.
render_panel_gateway() {
 render_legacy_panel_gateway "$@" | awk '
 /^[[:space:]]*server \{/ {
  print "  map $uri $kore_frame_options {"
  print "    default DENY;"
  print "    ~^/api/v1/playback/[A-Za-z0-9_.-]+/[A-Za-z0-9_-]+/(embed[.]html|embed[.]js|hls[.]js)$ \"\";"
  print "  }"
 }
 {gsub(/add_header X-Frame-Options DENY always;/,"add_header X-Frame-Options $kore_frame_options always;");print}
 '
}
# Probe the reviewed public TLS identity through loopback, without browser state,
# proxy environment or ~/.curlrc. Never weaken certificate verification.
# Caller may print this bounded diagnostic; no response body or raw curl error.
check_panel_https() {
  local host=$1 port=$2 certificate=$3 attempts=${4:-20} delay=${5:-1}
  local origin tls_host=$host status code attempt
  GATEWAY_CHECK_ERROR='parametros de verificacao invalidos'
  valid_gateway_port "$port" && [[ $attempts =~ ^[1-9][0-9]?$ ]] && ((attempts <= 30)) || return 1
  [[ $delay == 0 || $delay == 1 ]] || return 1
  origin=$(gateway_url "$host" "$port"); origin=${origin%/}
  if [[ $host == *:* ]]; then tls_host="[$host]"; fi
  for ((attempt=1; attempt<=attempts; attempt++)); do
    code=0
    status=$(curl -q --noproxy '*' --globoff --silent \
      --cacert "$certificate" --connect-to "$tls_host:$port:127.0.0.1:$port" \
      --connect-timeout 2 --max-time 3 --header 'Authorization:' --header 'Cookie:' \
      -o /dev/null -w '%{http_code}' "$origin/api/v1/auth/me" 2>/dev/null) || code=$?
    if [[ $code == 0 && $status == 401 ]]; then GATEWAY_CHECK_ERROR=''; return 0; fi
    case "$code:$status" in
      0:502|0:503|7:*|28:*|52:*|56:*)
        GATEWAY_CHECK_ERROR="gateway ainda indisponivel (curl=$code, HTTP=${status:-000}, tentativa=$attempt/$attempts)" ;;
      60:*) GATEWAY_CHECK_ERROR='certificado TLS recusado: confira validade e SAN para o dominio/IP confirmado (curl=60)'; return 1 ;;
      77:*) GATEWAY_CHECK_ERROR='nao foi possivel ler o certificado local confiavel (curl=77)'; return 1 ;;
      0:*)
        if [[ $status =~ ^[0-9]{3}$ ]]; then GATEWAY_CHECK_ERROR="resposta HTTPS inesperada: HTTP=$status; esperado 401 sem sessao"
        else GATEWAY_CHECK_ERROR='resposta HTTPS invalida'; fi
        return 1 ;;
      *) GATEWAY_CHECK_ERROR="falha de transporte HTTPS (curl=$code)"; return 1 ;;
    esac
    if ((attempt < attempts)); then sleep "$delay"; fi
  done
  return 1
}

# Test the public bootstrap as a browser does, including Origin on module scripts.
check_panel_assets() {
  local host=$1 port=$2 certificate=$3 origin tls_host=$1 html module status
  GATEWAY_CHECK_ERROR='HTML ou arquivos do login indisponiveis'
  origin=$(gateway_url "$host" "$port"); origin=${origin%/}
  if [[ $host == *:* ]]; then tls_host="[$host]"; fi
  local -a options=(-q --noproxy '*' --globoff --silent --fail --cacert "$certificate"
    --connect-to "$tls_host:$port:127.0.0.1:$port" --connect-timeout 2 --max-time 5
    --header 'Authorization:' --header 'Cookie:')
  html=$(curl "${options[@]}" --max-filesize 65536 "$origin/" 2>/dev/null) || return 1
  if [[ $html =~ src=\"(/ui/assets/[a-zA-Z0-9_-]+\.js)\" ]]; then module=${BASH_REMATCH[1]}
  else GATEWAY_CHECK_ERROR='HTML do login sem modulo JavaScript reconhecido'; return 1; fi
  status=$(curl "${options[@]}" --header "Origin: $origin" --header 'Sec-Fetch-Site: same-origin' \
    --header 'Sec-Fetch-Mode: cors' --header 'Sec-Fetch-Dest: script' --max-filesize 2097152 \
    -o /dev/null -w '%{http_code}:%{content_type}' "$origin$module" 2>/dev/null) || return 1
  if [[ $status =~ ^200:(text|application)/(javascript|ecmascript)(\;.*)?$ ]]; then
    GATEWAY_CHECK_ERROR=''; return 0
  fi
  GATEWAY_CHECK_ERROR='JavaScript do login recusado ou com tipo MIME incorreto'
  return 1
}
# A single successful readiness response is insufficient while ingests start.
# Require one unchanged core process and both units throughout the observation.
check_panel_stability(){
 local backend=$1 pid current attempt
 pid=$(systemctl show kore-edge.service -p MainPID --value) || return 1
 [[ $pid =~ ^[1-9][0-9]*$ ]] || return 1
 for attempt in {1..30}; do
  systemctl is-active --quiet kore-edge.service kore-edge-gateway.service || return 1
  current=$(systemctl show kore-edge.service -p MainPID --value) || return 1
  [[ $current == "$pid" ]] || return 1
  curl --noproxy '*' --globoff --silent --fail --max-time 2 "http://$backend/readyz" >/dev/null || return 1
  sleep 1
 done
}
