# Capacidade de canais - revisao operacional

O perfil `deploy/systemd/kore-edge-capacity.conf.example` e uma proposta, nao e instalado automaticamente. Foi dimensionado para o Edge de teste medido com 28 CPUs logicas e aproximadamente 32 GiB RAM: 128 recepcoes, 40 entradas HLS, 16 saidas HLS, limite de memoria 4 GiB, 512 tarefas e ate quatro CPUs. Isso nao garante banda, disponibilidade da origem ou codecs compativeis. TS nao exige segmentacao HLS; HLS e UDP continuam respeitando as permissoes da matriz. Nao ha transcodificacao automatica.

## Pre-verificacao (somente leitura)

No Edge de teste, confirmar identidade do host e executar:

```sh
nproc
free -h
systemctl show kore-edge.service -p MemoryMax -p TasksMax -p MemoryCurrent -p TasksCurrent -p NRestarts
systemctl cat kore-edge.service
curl --fail http://127.0.0.1:8080/readyz
```

Confirmar o endereco real de prontidao na configuracao antes de usar o exemplo curl. Conferir sobreposicoes de recursos existentes e espaco livre. Nao aplicar em producao.

## Aplicacao, somente apos aprovacao especifica

Salvar o conteudo anterior de `/etc/systemd/system/kore-edge.service.d/capacity.conf` em backup privado, ou registrar que nao existia. Se existir configuracao personalizada, revisar antes de substituir. Copiar o perfil revisado para esse destino, executar `systemctl daemon-reload` e `systemctl restart kore-edge.service`. O gateway nao precisa ser alterado. Nao ha mudancas de interface, rotas ou firewall.

## Validacao e rollback

Validar `/readyz`, estabilidade do PID por pelo menos 30 segundos, `NRestarts`, memoria e tarefas atuais, logs sem segredos e contagens de recepcao/espera. Comparar as contagens reais e investigar erros de origem separadamente. O aceite sintetico inclui 113 fontes recebendo simultaneamente com capacidade 128, cancelamento de fila, parada manual persistente e fonte saudavel recebendo apesar das demais em backoff.

Se houver falha, restaurar o arquivo anterior, ou remover somente o novo arquivo caso nao existisse, executar `systemctl daemon-reload` e reiniciar somente o core. Restaurar tambem a release anterior pelo backup do atualizador caso a falha seja de codigo. Verificar prontidao e estabilidade novamente. A atualizacao de binario sozinha mantem os limites anteriores; o perfil exige aprovacao independente.
