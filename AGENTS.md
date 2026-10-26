# Instruções permanentes para Codex

Este repositório é o Kore Edge. Leia README e arquitetura antes de editar.

- Trabalhe somente no módulo solicitado, sem implementar funcionalidades adjacentes.
- Segurança de produção acima de conveniência: nunca tocar em hosts de produção.
- Não executar `apt install`, `systemctl`, `ip route`, `wg-quick`, `ufw`, `iptables`, `nft` ou alterações de NIC sem aprovação humana específica.
- Antes de qualquer mudança operacional, entregar comandos de pré-verificação, plano de rollback e validação pós-mudança.
- Nunca gravar credenciais, chaves privadas, tokens ou dados de clientes em arquivos versionados ou logs.
- Usar streams sintéticos para testes; mockar a matriz.
- Preferir código testável, dependências mínimas, timeouts, limites de recursos, backoff e idempotência.
- Nunca assumir interface de rede, IP, gateway ou porta; detectar e pedir confirmação.
- Diferenciar métricas reais, indisponíveis e simuladas.
- Não transcodificar por padrão. Verificar codecs para HLS.
- Todo módulo exige testes, README atualizado e critérios de aceite atendidos.
