# Links de reprodução para VLC

## Uso

1. Selecione um canal recebendo em **Canais** e abra **Saídas**.
2. Clique em **Gerar links para VLC** e **Copiar link TS**.
3. No VLC, abra **Mídia → Abrir fluxo de rede**, ou Ctrl+N, e cole o link.
4. HLS usa o segundo link, quando autorizado/configurado e com segmentos prontos.
5. **Lista TS/HLS** baixa M3U das saídas prontas no filtro atual.

O player não precisa do cookie do navegador. Validade: sete dias, indicada na
tela. **Revogar links deste canal** invalida os links anteriores; gere outros se
precisar. A revogação encerra players externos, sem parar o canal ou o mini player.
Não envie esses links em tickets ou logs: eles contêm uma chave de reprodução.

## Se não tocar

- Endereços antigos `/api/v1/live/...` são internos do painel e retornam 401 sem
  sessão. Gere o link externo `/api/v1/playback/...`.
- Use TS primeiro. HLS depende de grant, codecs H.264/AAC, ferramentas e vagas
  de segmentação. Gerar um link não inicia recepção nem habilita HLS.
- Use o domínio/IP HTTPS correto e um certificado confiado pelo cliente. O
  certificado local inicial precisa ser confiado no computador do VLC; não
  desative TLS e não exponha HTTP para contornar o problema.
- 401: link expirado/revogado, administrador emissor alterado/desabilitado ou
  grant do canal indisponível. 503/404: recepção/segmentos ainda indisponíveis.
- Há dois consumidores por link e 32 globais. Excedentes recebem 429.

## Contrato

`POST /api/v1/live/playback-links` recebe `action: issue|revoke` e `channel_ids`.
Exige administrador autenticado, Origin válido, CSRF e auditoria. Não exige nova
senha. Emissão só aceita canais configurados com autorização vigente. O retorno
contém caminhos relativos TS/HLS e validade. Sem HLS configurado/autorizado, não
retorna link HLS. Emissão é limitada a 128 IDs e ao limite de 8 KiB da API.

`GET|HEAD /api/v1/playback/<capability>/<channel>/stream.ts|index.m3u8|seg*.ts`
verifica HMAC, canal, prazo, owner/revisão e revogação. Não concede sessão nem
permite chamar APIs administrativas. Não aceita consultas, caminhos codificados,
traversal ou origem web diferente. Acesso passa pelo gateway HTTPS existente,
com logs de acesso desativados; não se registram URLs de capabilities.

O segredo e as gerações são privados, externos ao checkout e com escrita atômica.
As capacidades completas não são gravadas pelo servidor. Sobrevivem ao restart
e logout do navegador, mas mudança de senha/revisão ou bloqueio do administrador
emissor as invalida. Cada requisição HLS verifica autorização e cada conexão TS
é revalidada a cada segundo. Segmentos relativos mantêm o diretório autenticado.
Usa a recepção existente, sem iniciar fontes ou alterar NIC/firewall/serviços.

## Aceite

Testes sintéticos cobrem emissão protegida por sessão/CSRF, escopo, adulteração,
expiração, revogação persistida, limites de consumidores, GET HTTPS sem cookie,
enquadramento TS, encerramento de conexão revogada e manutenção do ingest.
O teste nativo opcional exige FFmpeg/ffprobe portáteis e valida playlist HLS,
segmentos sem cookie e codecs H.264/AAC. A CI o executa com detector de corrida.
Não usa fontes reais nem pressupõe que o certificado da VPS seja confiado no VLC.
