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
