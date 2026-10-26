#!/usr/bin/env bash
# Local operator recovery. Never accepts a password in argv or environment.
set -Eeuo pipefail
umask 077
if [[ ${1:-} == --plan || ${1:-} == --help ]]; then
  printf '%s\n' 'Uso: sudo bash scripts/panel-access.sh' \
    'Verifica usuario/arquivo local e permite conferir a senha sem alteracao.' \
    'Redefinicao exige escolha e confirmacao; backup privado e troca atomica.' \
    'Nao reinicia servicos; nao altera certificados, rede ou configuracao da API.'
  exit 0
fi
(($#==0)) || { printf '%s\n' 'Opcao desconhecida.' >&2; exit 1; }
((EUID==0)) || { printf '%s\n' 'Execute com sudo: sudo bash scripts/panel-access.sh' >&2; exit 1; }
[[ -t 0 && -t 1 ]] || { printf '%s\n' 'Execute em um terminal interativo; senha por pipe/argumento nao e aceita.' >&2; exit 1; }
die(){ printf 'ERRO: %s\n' "$1" >&2; exit 1; }
for command in htpasswd runuser getent flock; do command -v "$command" >/dev/null || die 'Ferramenta de autenticacao ausente.'; done
if [[ -f /var/lib/kore-edge/users/users.json ]]; then
 exec bash "$(dirname -- "${BASH_SOURCE[0]}")/reset-admin.sh" --apply
fi
directory=/etc/kore-edge/gateway
file="$directory/htpasswd"
for path in /etc/kore-edge "$directory" "$file" /run/kore-edge-installer.lock; do [[ ! -L $path ]] || die 'Destino com symlink; operacao recusada.'; done
[[ -f $file && -f $directory/nginx.conf ]] || die 'Gateway instalado nao encontrado.'
grep -Fqx '    auth_basic_user_file /etc/kore-edge/gateway/htpasswd;' "$directory/nginx.conf" || die 'Arquivo de autenticacao diverge da configuracao.'
[[ $(wc -l <"$file") == 1 ]] || die 'Esperado um unico usuario; arquivo deve ser revisado manualmente.'
IFS=: read -r username stored <"$file"
[[ $username =~ ^[a-zA-Z][a-zA-Z0-9_-]{0,31}$ && $stored == '$2'* ]] || die 'Formato da credencial inesperado.'
unset stored
[[ $(id -u kore-edge) -gt 0 ]] || die 'Conta do gateway invalida.'
exec 9>/run/kore-edge-installer.lock
flock -n 9 || die 'Outro instalador/reparo esta ativo.'
printf '\n=== Acesso ao painel Kore Edge ===\nUsuario cadastrado: %s\n' "$username"
printf '%s\n' 'No navegador use este usuario, nao necessariamente seu usuario do Ubuntu.'
if ! runuser -u kore-edge -- test -r "$file"; then
  printf '%s\n' 'O usuario do servico nao consegue ler a credencial. A redefinicao criara o arquivo privado com o dono correto.'
fi
printf '\n1) Conferir uma senha (sem alterar)\n2) Cadastrar nova senha\n3) Sair\n'
IFS= read -r -p 'Escolha [Enter = sair]: ' choice || exit 0
case "$choice" in
  1)
    read -r -s -p 'Digite a senha a conferir: ' password || exit 0; printf '\n'
    if printf '%s\n' "$password" | htpasswd -vi "$file" "$username" >/dev/null 2>&1; then
      printf '%s\n' 'Senha confere com o cadastro. Abra uma janela privativa e use o usuario mostrado acima.'
    else
      printf '%s\n' 'Nao foi possivel validar essa senha. Execute novamente e escolha 2 para redefinir.'
    fi
    unset password; exit 0 ;;
  2) ;;
  *) printf '%s\n' 'Encerrado sem alteracoes.'; exit 0 ;;
esac
printf '%s\n' 'Sera alterada somente a senha deste usuario. Certificado, portas e servicos serao preservados.'
printf '%s\n' 'Um backup privado permitira restauracao manual da credencial anterior, se necessario.'
read -r -p 'Confirmar redefinicao? [s/N]: ' confirmation || exit 0
[[ $confirmation == s || $confirmation == S ]] || exit 0
while :; do
  read -r -s -p 'Nova senha (12 a 72 caracteres; ate 72 bytes): ' password || exit 0; printf '\n'
  read -r -s -p 'Repita a nova senha: ' repeated || exit 0; printf '\n'
  bytes=$(printf '%s' "$password" | LC_ALL=C wc -c)
  if [[ ${#password} -ge 12 && $bytes -le 72 && $password == "$repeated" && $password != *:* ]]; then break; fi
  printf '%s\n' 'Senhas diferentes ou tamanho invalido; nao use dois-pontos.'
done
unset repeated
candidate=$(mktemp "$directory/.htpasswd-new.XXXXXXXX")
trap '[[ ! -e $candidate ]] || rm -- "$candidate"' EXIT
printf '%s\n' "$password" | htpasswd -iBc -C 10 "$candidate" "$username" >/dev/null 2>&1 || die 'Nao foi possivel gerar a credencial.'
printf '%s\n' "$password" | htpasswd -vi "$candidate" "$username" >/dev/null 2>&1 || die 'A verificacao da nova credencial falhou; arquivo anterior preservado.'
unset password
chown kore-edge:kore-edge "$candidate"; chmod 0600 "$candidate"
backup=$(mktemp "$directory/htpasswd.before-reset.XXXXXXXX")
cp --preserve=mode,ownership "$file" "$backup"
chmod 0600 "$backup"
mv -T -- "$candidate" "$file"
printf 'Senha atualizada para %s. Backup privado: %s\n' "$username" "$backup"
printf '%s\n' 'Abra uma janela privativa do navegador e entre com esse usuario e a nova senha. Nao precisa reiniciar o Nginx.'
