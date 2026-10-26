#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
cat <<'PLAN'
Recuperacao local de administrador Kore Edge.
Pre-verificacao: root em terminal, conta existente, arquivos privados, sem symlinks.
Mudancas: parar somente kore-edge, backup privado da base, redefinir o hash de um
administrador ativo, revogar suas sessoes e iniciar novamente kore-edge.
Rollback: se a gravacao falhar, restaurar a base salva antes de reiniciar o core.
Pos-validacao: prontidao local e login manual com a nova senha no navegador.
Sem mudanca de certificado, gateway, portas, firewall ou rede.
PLAN
case "${1:---plan}" in --plan|--help) exit 0 ;; --apply) ;; *) exit 1 ;; esac
(($#==1)) || exit 1
die(){ printf 'ERRO: %s\n' "$1" >&2; exit 1; }
((EUID==0)) && [[ -t 0 && -t 1 ]] || die 'Use sudo e terminal interativo.'
for command in htpasswd runuser flock systemctl; do command -v "$command" >/dev/null || die 'Dependencia ausente.'; done
for path in /var/lib/kore-edge /var/lib/kore-edge/users /var/lib/kore-edge/users/users.json /run/kore-edge-installer.lock; do [[ ! -L $path ]] || die 'Symlink inesperado.'; done
[[ -f /var/lib/kore-edge/users/users.json && -x /opt/kore-edge/current/edge-users ]] || die 'Modulo de usuarios nao instalado.'
exec 9>/run/kore-edge-installer.lock
flock -n 9 || die 'Outra operacao ativa.'
systemctl is-active --quiet kore-edge.service || die 'Core deve estar ativo antes da recuperacao.'
read -r -p 'Usuario administrador existente: ' username
[[ $username =~ ^[a-zA-Z0-9][a-zA-Z0-9_.@+-]{0,127}$ ]] || die 'Usuario invalido.'
read -r -p 'Confirma interrupcao breve e redefinicao? Digite REDEFINIR: ' answer
[[ $answer == REDEFINIR ]] || exit 0
while :; do
 read -r -s -p 'Nova senha (12 a 72 bytes): ' password; printf '\n'
 read -r -s -p 'Repita a nova senha: ' repeated; printf '\n'
 bytes=$(printf '%s' "$password" | LC_ALL=C wc -c)
 [[ $bytes -ge 12 && $bytes -le 72 && $password == "$repeated" ]] && break
 printf 'Tamanho invalido ou senhas diferentes.\n'
done
unset repeated
work=$(mktemp -d /var/lib/kore-edge/.recovery.XXXXXXXX)
chown kore-edge:kore-edge "$work"; chmod 0700 "$work"
trap 'rm -rf -- "$work"' EXIT
printf '%s\n' "$password" | htpasswd -iBc -C 12 "$work/htpasswd" "$username" >/dev/null 2>&1
printf '%s\n' "$password" | htpasswd -vi "$work/htpasswd" "$username" >/dev/null 2>&1
unset password
chown kore-edge:kore-edge "$work/htpasswd"; chmod 0600 "$work/htpasswd"
backup=$(mktemp -d /var/lib/kore-edge/users-before-reset.XXXXXXXX)
printf 'Backup privado: %s\n' "$backup"
stopped=0
recover(){ local status=$?; if ((stopped)); then systemctl start kore-edge.service || true; fi; rm -rf -- "$work"; exit "$status"; }
trap recover EXIT
systemctl stop kore-edge.service
stopped=1
cp -p /var/lib/kore-edge/users/users.json "$backup/users.json"
if ! runuser -u kore-edge -- /opt/kore-edge/current/edge-users --action reset-htpasswd --directory /var/lib/kore-edge/users --file "$work/htpasswd"; then
 cp -p "$backup/users.json" /var/lib/kore-edge/users/users.json
 die 'Redefinicao recusada; base anterior restaurada.'
fi
systemctl start kore-edge.service
stopped=0
printf 'Senha alterada. Entre novamente no navegador.\n'
