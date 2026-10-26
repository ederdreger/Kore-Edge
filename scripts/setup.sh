#!/usr/bin/env bash
# Guided front end. All actual mutations remain in the reviewed installer.
set -Eeuo pipefail
installer="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)/install.sh"
if (($#)); then
  if [[ $1 == --help && $# == 1 ]]; then
    printf '%s\n' 'Uso: bash scripts/setup.sh' 'Responda as etapas. Nada sera instalado sem sua escolha final.'
    exit 0
  fi
  printf '%s\n' 'Opcao desconhecida. Execute: bash scripts/setup.sh' >&2
  exit 1
fi
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

printf '\nETAPA 1/5 - Servidor de laboratorio\n'
ask 'Este servidor e exclusivo para laboratorio/testes? [s/N]:'
case "$answer" in s|S|sim|SIM) ;; *) printf '%s\n' 'Cancelado. Nenhuma alteracao realizada.'; exit 0 ;; esac

printf '\nETAPA 2/5 - O que instalar\n'
printf '%s\n' '1) Completo: painel, sistema, FFmpeg e ferramentas WireGuard (recomendado)' '2) Basico: painel e API'
while :; do
  ask 'Escolha [1/2, Enter = 1]:'
  case "$answer" in ''|1) profile=media; break ;; 2) profile=core; break ;; *) printf '%s\n' 'Digite 1 ou 2.' ;; esac
done

printf '\nETAPA 3/5 - Acesso ao painel\n'
printf '%s\n' 'O painel ficara acessivel somente neste servidor (loopback).'
printf '%s\n' 'Para acessar de outro computador, use o tunel SSH explicado no guia simples.'
addresses=()
if command -v ip >/dev/null 2>&1; then
  while IFS= read -r candidate; do
    case "$candidate" in
      127.*|::1) addresses+=("$candidate") ;;
    esac
  done < <(ip -o address show scope host 2>/dev/null | awk '{split($4,a,"/"); print a[1]}' | sort -u)
fi
if ((${#addresses[@]})); then
  printf '%s\n' 'Enderecos locais detectados:'
  for index in "${!addresses[@]}"; do printf '%d) %s\n' "$((index+1))" "${addresses[$index]}"; done
  while :; do
    ask 'Escolha o numero do endereco local:'
    if [[ $answer =~ ^[1-9][0-9]{0,2}$ ]] && ((answer <= ${#addresses[@]})); then
      address=${addresses[$((answer-1))]}; break
    fi
    printf '%s\n' 'Selecione um dos numeros exibidos.'
  done
else
  printf '%s\n' 'Nao foi possivel detectar o loopback. Confira o endereco local no servidor.'
  ask 'Digite o IP de loopback confirmado (IPv4 127.x.x.x ou ::1):'
  address=$answer
fi
while :; do
  ask 'Qual porta deseja usar para o painel? (exemplo: 18080):'
  port=$answer
  if [[ $address == ::1 ]]; then listen="[::1]:$port"; else listen="$address:$port"; fi
  # Reuse the installer's address/port validation without any host mutation.
  if bash "$installer" --plan --profile "$profile" --listen "$listen" >/dev/null 2>&1; then
    if command -v ss >/dev/null 2>&1 && [[ $port =~ ^[0-9]+$ ]]; then
      if ss -H -ltn "sport = :$port" 2>/dev/null | grep -q .; then
        printf '%s\n' 'Essa porta esta ocupada. Escolha outra.'
        continue
      fi
    fi
    break
  fi
  printf '%s\n' 'Endereco ou porta invalidos. A porta deve ser de 1 a 65535, sem zero inicial.'
  if ((${#addresses[@]} == 0)); then
    ask 'Digite novamente o IP de loopback confirmado:'
    address=$answer
  fi
done

printf '\nETAPA 4/5 - Inicio do painel\n'
while :; do
  ask 'Deseja iniciar o painel ao terminar a instalacao? [s/N]:'
  case "$answer" in s|S|sim|SIM) start=1; break ;; ''|n|N|nao|NAO) start=0; break ;; *) printf '%s\n' 'Responda s ou n.' ;; esac
done

printf '\nETAPA 5/5 - Resumo e confirmacao\n'
bash "$installer" --plan --profile "$profile" --listen "$listen"
if ((start)); then printf '%s\n' 'Inicio solicitado: SIM, somente o novo servico.'; else printf '%s\n' 'Inicio solicitado: NAO, apenas instalar os arquivos.'; fi
printf '\nEndereco escolhido para o painel: http://%s/\n' "$listen"
printf '%s\n' 'Pacotes existentes nao serao removidos. Arquivos existentes nao serao sobrescritos.'
printf '\n1) Instalar agora no laboratorio\n2) Encerrar somente com a simulacao\n3) Cancelar\n'
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
arguments=(--apply --lab-only --profile "$profile" --listen "$listen")
if ((start)); then arguments+=(--start); fi
printf '\nIniciando instalacao. Aguarde a verificacao, os pacotes e os testes.\n'
if ((EUID == 0)); then exec bash "$installer" "${arguments[@]}"; fi
command -v sudo >/dev/null || { printf '%s\n' 'sudo indisponivel. Execute o assistente com a conta administradora.' >&2; exit 1; }
printf '%s\n' 'O sudo pedira a senha do seu usuario administrador, se necessario.'
exec sudo -- bash "$installer" "${arguments[@]}"
