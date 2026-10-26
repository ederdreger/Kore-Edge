#!/usr/bin/env bash
# Guided front end. All actual mutations remain in the reviewed installer.
set -Eeuo pipefail
installer="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)/install.sh"
if (($#)); then
  if [[ $1 == --help && $# == 1 ]]; then
  printf '%s\n' 'Uso: bash scripts/setup.sh [--resume-packages]' 'Responda as etapas. Nada sera instalado sem sua escolha final.'
    exit 0
  fi
  if [[ $1 != --resume-packages || $# != 1 ]]; then
    printf '%s\n' 'Opcao desconhecida. Execute: bash scripts/setup.sh' >&2
    exit 1
  fi
fi
resume_args=()
if [[ ${1:-} == --resume-packages ]]; then resume_args=(--resume-packages); fi
source "$(dirname -- "$installer")/gateway-lib.sh"
answer=''
ask() {
  printf '\n%s ' "$1"
  if ! IFS= read -r answer; then
    printf '\nEntrada encerrada. Instalacao cancelada; nenhuma alteracao realizada.\n'
    exit 0
  fi
}
printf '\n=== Kore Edge | Instalacao passo a passo ===\n'
printf '%s\n' 'Voce vai escolher os dados; ao final podera instalar, simular ou cancelar.'
printf '%s\n' 'O sistema ainda esta em homologacao e nao deve ser instalado em producao.'

printf '\nETAPA 1/5 - Instalacao limpa\n'
printf '%s\n' 'Sem canais de exemplo, trafego simulado ou credenciais padrao.'
printf '%s\n' 'A instalacao exige revisao do ambiente antes de disponibilizar canais reais.'
ask 'Preparar instalacao limpa neste servidor? [s/N]:'
case "$answer" in s|S|sim|SIM) ;; *) printf '%s\n' 'Cancelado. Nenhuma alteracao realizada.'; exit 0 ;; esac
mode_args=(--production)

printf '\nETAPA 2/5 - O que instalar\n'
printf '%s\n' '1) Completo: painel, sistema, FFmpeg e ferramentas WireGuard (recomendado)' '2) Basico: painel e API'
while :; do
  ask 'Escolha [1/2, Enter = 1]:'
  case "$answer" in ''|1) profile=media; break ;; 2) profile=core; break ;; *) printf '%s\n' 'Digite 1 ou 2.' ;; esac
done

printf '\nETAPA 3/5 - Acesso ao painel\n'
printf '%s\n' 'Informe o dominio apontado para a VPS ou o IP da VPS (sem http://, porta ou caminho).'
addresses=()
if command -v ip >/dev/null 2>&1; then
  while IFS= read -r candidate; do
    candidate=${candidate,,}
    if valid_public_host "$candidate"; then addresses+=("$candidate"); fi
  done < <(ip -o address show up scope global 2>/dev/null | awk '{split($4,a,"/"); print a[1]}' | sort -u)
fi
if ((${#addresses[@]})); then
  printf '%s\n' 'IPs detectados nas interfaces (confirme o endereco desejado):'
  for index in "${!addresses[@]}"; do
    printf '%d) %s - %s\n' "$((index+1))" "${addresses[$index]}" "$(gateway_host_scope "${addresses[$index]}")"
  done
fi
while :; do
  ask 'Dominio ou IP da VPS (ou numero da lista):'
  public_host=${answer,,}
  if [[ $answer =~ ^[1-9][0-9]{0,2}$ ]] && ((answer <= ${#addresses[@]})); then public_host=${addresses[$((answer-1))]}; fi
  public_host=${public_host#[}; public_host=${public_host%]}
  if valid_public_host "$public_host"; then break; fi
  printf '%s\n' 'Endereco invalido. Use dominio, IPv4 ou IPv6 da VPS; nao use localhost/loopback.'
done
printf 'Tipo detectado: %s\n' "$(gateway_host_scope "$public_host")"
case "$(gateway_host_scope "$public_host")" in
  *privado*|*CGNAT*) printf '%s\n' 'Esse IP e aceito. O acesso funciona na rede privada/VPN; para internet use dominio/IP externo com NAT configurado.' ;;
esac
while :; do
  ask 'Porta HTTPS do painel (exemplo: 443 ou 8443):'
  public_port=$answer
  if valid_gateway_port "$public_port"; then
    if command -v ss >/dev/null 2>&1 && ss -H -ltn "sport = :$public_port" 2>/dev/null | grep -q .; then
      printf '%s\n' 'Essa porta esta ocupada. Escolha outra.'; continue
    fi
    break
  fi
  printf '%s\n' 'Porta invalida. Digite um numero de 1 a 65535.'
done
while :; do
  ask 'Usuario para entrar no painel [Enter = admin]:'
  gateway_user=${answer:-admin}
  [[ $gateway_user =~ ^[a-zA-Z][a-zA-Z0-9_-]{0,31}$ ]] && break
  printf '%s\n' 'Use letras, numeros, hifen ou underscore, iniciando por letra (ate 32 caracteres).'
done
printf '%s\n' 'A senha sera solicitada com entrada oculta ao aplicar a instalacao.'
printf '%s\n' 'HTTPS sera protegido por certificado local; o navegador exibira aviso de confianca.'
printf '%s\n' 'Para dominio, o DNS deve apontar para esta VPS. A porta HTTPS precisa estar permitida no provedor/firewall.'
# Select a free private upstream port; the operator confirms it in the summary.
port=18080
while ((port < 18180)); do
  if [[ $port == "$public_port" ]]; then port=$((port+1)); continue; fi
  if command -v ss >/dev/null 2>&1 && ss -H -ltn "sport = :$port" 2>/dev/null | grep -q .; then
    port=$((port+1)); continue
  fi
  break
done
((port < 18180)) || { printf '%s\n' 'Nao foi encontrada uma porta interna livre.' >&2; exit 1; }
listen="127.0.0.1:$port"
public_args=(--public-host "$public_host" --public-port "$public_port" --gateway-user "$gateway_user")

printf '\nETAPA 4/5 - Inicio do painel\n'
while :; do
  ask 'Deseja iniciar o painel ao terminar a instalacao? [s/N]:'
  case "$answer" in s|S|sim|SIM) start=1; break ;; ''|n|N|nao|NAO) start=0; break ;; *) printf '%s\n' 'Responda s ou n.' ;; esac
done

printf '\nETAPA 5/5 - Resumo e confirmacao\n'
bash "$installer" --plan "${mode_args[@]}" --profile "$profile" --listen "$listen" "${public_args[@]}" "${resume_args[@]}"
if ((start)); then printf '%s\n' 'Inicio solicitado: SIM, somente o novo servico.'; else printf '%s\n' 'Inicio solicitado: NAO, apenas instalar os arquivos.'; fi
printf '\nPainel da VPS: %s\n' "$(gateway_url "$public_host" "$public_port")"
printf '%s\n' 'Pacotes existentes nao serao removidos. Arquivos existentes nao serao sobrescritos.'
printf '\n1) Instalar agora\n2) Encerrar somente com a simulacao\n3) Cancelar\n'
while :; do
  ask 'Sua escolha [1/2/3, Enter = 2]:'
  case "$answer" in
    ''|2) printf '%s\n' 'Simulacao concluida. Nenhuma alteracao realizada.'; exit 0 ;;
    3) printf '%s\n' 'Cancelado. Nenhuma alteracao realizada.'; exit 0 ;;
    1) break ;;
    *) printf '%s\n' 'Digite 1, 2 ou 3.' ;;
  esac
done
if [[ ! -t 0 || ! -t 1 ]]; then
  printf '%s\n' 'Para instalar, execute o assistente em um terminal interativo. Entrada por pipe nao instala.' >&2
  exit 1
fi
arguments=(--apply "${mode_args[@]}" --profile "$profile" --listen "$listen" "${public_args[@]}" "${resume_args[@]}")
if ((start)); then arguments+=(--start); fi
printf '\nIniciando instalacao. Aguarde a verificacao, os pacotes e os testes.\n'
if ((EUID == 0)); then exec bash "$installer" "${arguments[@]}"; fi
command -v sudo >/dev/null || { printf '%s\n' 'sudo indisponivel. Execute o assistente com a conta administradora.' >&2; exit 1; }
printf '%s\n' 'O sudo pedira a senha do seu usuario administrador, se necessario.'
exec sudo -- bash "$installer" "${arguments[@]}"
