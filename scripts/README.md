`setup.sh` é o assistente simples: execute `bash scripts/setup.sh` e responda às
cinco etapas. Veja [o passo a passo](../docs/INSTALACAO-SIMPLES.md). A instalação
só ocorre após seleção final em terminal interativo; Enter apenas simula.

`install.sh` é o instalador por parâmetros para laboratório Ubuntu 24.04. Por padrão só
imprime o plano. A aplicação exige `--apply --lab-only` e listener explícito;
`--start` solicita iniciar apenas o novo serviço, sem habilitar boot.

Não execute em produção. Consulte [INSTALL.md](../docs/INSTALL.md) para pacotes,
pré-verificação, impactos, validação e rollback. Não há alteração automática de
NIC, firewall, rotas ou túneis. O script não foi aplicado no host de desenvolvimento.
