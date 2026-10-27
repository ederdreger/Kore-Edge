# Revisao de estabilidade da recepcao

Escopo: prazo inicial de resposta da matriz e classificacao segura das falhas. O prazo anterior de 3 segundos podia interromper uma origem dentro dos 5 segundos permitidos pela matriz. O prazo revisado e 8 segundos. O watchdog de pacotes continua ativo; nenhum fluxo recebe prazo total de duracao.

## Pre-verificacao no Edge de teste confirmado

```sh
systemctl show kore-edge.service -p MainPID -p NRestarts -p MemoryCurrent -p TasksCurrent
curl --fail --silent http://127.0.0.1:18080/readyz
bash scripts/update-panel.sh --plan
```

Confirmar a porta no gateway instalado antes de executar o exemplo de prontidao. Verificar perfil de capacidade atual e checkout limpo. Nao executar em hosts de producao.

## Aplicacao apos aprovacao especifica

Usar o atualizador existente `bash scripts/update-panel.sh --apply`. Ele compila e testa antes da troca, cria backup privado de configuracao e distribuicao, preserva a release atual e reinicia somente core/gateway. O perfil de capacidade existente fica preservado. Uma falha de validacao restaura automaticamente configuracao, distribuicao e release anteriores. Para reversao posterior, usar os caminhos exatos informados pelo atualizador, com os servicos autorizados parados; nao inventar nomes de backup.

## Pos-validacao

Confirmar prontidao, mesmo PID estavel por 30 segundos e ausencia de reinicios inesperados. Acompanhar o canal solicitado na aba Qualidade, incluindo ultimo motivo da queda. Comparar interrupcoes dentro da mesma ativacao: o contador reinicia quando o processo e reiniciado. Consultar somente as linhas sanitizadas:

```sh
journalctl -u kore-edge.service --since '5 minutes ago' --no-pager | grep 'live_reception_failures counts='
```

As contagens agregam protocolo e motivo, sem nomes, URLs ou credenciais. HTTP 429 indica limite da matriz; 503/504 indicam indisponibilidade/prazo remoto. `matrix_response_timeout` difere de `source_idle_timeout`. `source_closed`, transporte invalido e falhas de saida exigem diagnosticos distintos. Nenhuma dessas categorias, isoladamente, comprova estabilidade audiovisual prolongada.
