# Liberar e buscar canais por parceiro

## 1. No Kore Play: escolher quem recebe

1. Confirme que o Edge já está cadastrado e conectado em **Canais → WireGuard Kore Edge**.
2. Abra **Canais → Parceiros Kore Edge**.
3. Escolha **Novo parceiro**, informe o nome e selecione o servidor Edge correspondente.
4. Marque os canais desejados. A coluna **Protocolo de entrega** mostra o protocolo da origem cadastrada.
5. Informe a senha administrativa **do Kore Play**, confirme a seleção e clique em **Publicar canais para este parceiro**.

Para alterar a lista, selecione o parceiro existente. Desmarcar um canal retira sua autorização. Desabilitar o parceiro bloqueia todos os seus canais, inclusive entregas em andamento.

## 2. No Edge: buscar a lista

1. Abra **Canais**, onde aparece **Canais do Kore Play**.
2. Na primeira vez, confirme o endereço HTTPS da central e o IP privado dela dentro do túnel WireGuard. O domínio deve corresponder ao certificado HTTPS válido da central. O IP precisa pertencer às rotas já autorizadas no WireGuard.
3. Informe a senha administrativa **deste Edge** e clique em **Salvar central**.
4. Informe novamente a senha, confirme a consulta e clique em **Buscar canais**.
5. Confira o nome do parceiro, a quantidade de canais e a última busca. Com atualização automática habilitada, a lista é renovada a cada minuto.
6. Use **Testar sinal** em um canal piloto. A tela informa se uma amostra válida foi recebida e o horário da verificação.

Buscar a lista não inicia todos os canais. **Entrada de mídia autenticada** mostra o caminho protegido pela sessão do Edge, sem revelar a origem da central. Este módulo não conecta o catálogo real ao Stream Engine sintético nem cria saídas multicast automaticamente.

## Protocolos e qualidade

- **HTTP MPEG-TS**: entrega o fluxo HTTP/HTTPS MPEG-TS já existente, preservando os bytes recebidos.
- **HLS**: entrega a playlist e os segmentos já disponíveis na origem. Não converte TS em HLS nem altera codecs.
- **UDP e outras origens**: aparecem indisponíveis para esta entrega até existir um ingest compatível. Não são anunciados como HTTP/HLS artificialmente.

O teste TS verifica enquadramento em uma amostra curta. O teste HLS verifica a playlist; não comprova que todos os segmentos ou codecs funcionarão no player. Playlists com redirecionamento ou referências a outro domínio são recusadas neste módulo. Para HLS, valide áudio, vídeo e codecs no player de destino, conforme o [formato HLS](https://datatracker.ietf.org/doc/html/rfc8216).

Nenhum protocolo garante ausência de congelamentos ou quadriculação. Antes da operação completa, valide um canal piloto, banda disponível, perda, estabilidade da origem e compatibilidade do receptor. O painel distingue **origem cadastrada**, **amostra recebida** e **indisponível**.

## Se não aparecerem canais

- WireGuard online confirma o túnel, não a liberação de canais: publique as permissões no Kore Play.
- Confira se o parceiro está habilitado e vinculado ao Edge correto.
- Confira domínio HTTPS, certificado e IP privado da central. A consulta não usa a Internet como alternativa quando o túnel falha.
- Se a origem mudou de protocolo, revise a seleção no Kore Play e publique novamente.
- Se o catálogo expirou, refaça a busca. Uma falha de consulta não renova permissões vencidas.

As telas começam vazias; nenhum parceiro ou canal de demonstração é criado.
