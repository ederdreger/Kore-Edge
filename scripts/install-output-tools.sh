#!/usr/bin/env bash
# Optional, pinned MediaMTX companion. No APT, services or network changes.
set -Eeuo pipefail
umask 077
version=v1.21.3
digest=0a2ea7eba96023631efa1bea7804823a9145b043b6646a8a05798d4ac24ed635
target=/opt/kore-edge/tools/mediamtx-v1.21.3
link=/usr/local/bin/mediamtx
cat <<'PLAN'
Ferramenta opcional para saidas RTMP, RTSP e SRT do Kore Edge.
Pre-verificacao: Ubuntu/Linux amd64, curl, tar, sha256sum; destino sem symlink.
Mudanca: baixar MediaMTX v1.21.3 do projeto oficial, conferir SHA-256,
instalar binario/licenca em /opt/kore-edge/tools e link /usr/local/bin/mediamtx.
Nao executa APT, systemctl, firewall, NIC ou inicia listeners.
Rollback: remover somente o link instalado se ainda apontar para este destino;
preservar arquivos e configuracoes do Edge. Listeners continuam desativados.
Validacao: versao do binario; ativacao posterior somente no painel com IP/portas revisados.
PLAN
[[ ${1:---plan} == --apply ]] || { printf 'Simulacao. Para instalar no host de teste: --apply --lab-only\n'; exit 0; }
[[ $# == 2 && $2 == --lab-only && $EUID == 0 && $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { printf 'Exige root, Linux amd64 e --lab-only.\n' >&2; exit 1; }
for tool in curl tar sha256sum install; do command -v "$tool" >/dev/null; done
for path in /opt/kore-edge /opt/kore-edge/tools "$target" /usr/local/bin; do [[ ! -L $path ]] || { printf 'Destino com symlink inesperado.\n' >&2; exit 1; }; done
if [[ -e $link || -L $link ]]; then [[ -L $link && $(readlink "$link") == "$target/mediamtx" ]] || { printf 'MediaMTX existente fora deste instalador; preservar e revisar.\n' >&2; exit 1; }; fi
work=$(mktemp -d); trap 'rm -rf -- "$work"' EXIT
archive=$work/mediamtx.tar.gz
curl --fail --location --proto '=https' --tlsv1.2 --retry 3 --max-time 180 "https://github.com/bluenviron/mediamtx/releases/download/$version/mediamtx_${version}_linux_amd64.tar.gz" -o "$archive"
printf '%s  %s\n' "$digest" "$archive" | sha256sum --check --status
tar -xzf "$archive" -C "$work" mediamtx LICENSE
install -d -o root -g root -m 0755 "$target"
install -o root -g root -m 0755 "$work/mediamtx" "$target/mediamtx"
install -o root -g root -m 0644 "$work/LICENSE" "$target/LICENSE"
[[ -L $link ]] || ln -s "$target/mediamtx" "$link"
"$target/mediamtx" --version
printf 'Ferramenta instalada. Nenhum listener foi iniciado.\n'
