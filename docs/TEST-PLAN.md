# Plano de testes
- Unitários: parsing de config, validação de ACL, catálogo e API.
- Integração: stream sintético local, UDP unicast, multicast isolado, HLS e expiração de segmentos.
- Resiliência: túnel desconectado, fonte indisponível, jitter, perda, reinício do agente.
- Segurança: segredos, autorização por parceiro, token expirado, privilégios, isolamento de interface.
- Desempenho: medir CPU/RAM/IO, RX/TX, pacotes descartados, bitrate real e clientes HLS concorrentes.
- Aceite: não vazar multicast, não alterar produção, rollback demonstrado.
