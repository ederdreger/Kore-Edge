# Liberar e buscar canais por parceiro

## 1. No Kore Play: escolher quem recebe

1. Confirme que o Edge já está cadastrado e conectado em **Canais → WireGuard Kore Edge**.
2. Abra **Canais → Parceiros Kore Edge**.
3. Escolha **Novo parceiro**, informe o nome e selecione o servidor Edge correspondente. Confirme com a senha do Kore Play e clique em **Salvar parceiro e selecionar canais**.
4. A seleção de canais abre automaticamente. Marque os canais desejados e confira **Protocolo a enviar** em cada linha. Use a busca, o filtro HTTP MPEG-TS/HLS e **Selecionar canais desta busca** para marcar em lote. Somente protocolos existentes na origem são oferecidos; quando há apenas um, ele vem selecionado.
5. Informe a senha administrativa **do Kore Play**, confirme a seleção e clique em **Publicar canais para este parceiro**.

Para alterar a lista, selecione o parceiro existente. Desmarcar um canal retira sua autorização. Desabilitar o parceiro bloqueia todos os seus canais, inclusive entregas em andamento.

Se o parceiro já está salvo sem canais, clique em **Selecionar canais e protocolos
de [nome]** no cartão dele. Não precisa cadastrar novamente. O atalho **Selecionar
canais para este Edge** na tela WireGuard também abre o parceiro correspondente.
Use **Editar cadastro do parceiro** para alterar nome ou bloqueio e **Selecionar
canais e protocolos** para voltar à lista.

O painel distingue **canais na plataforma**, **resultados da busca** e **canais
selecionados para o parceiro**. Se aparecerem menos canais, use **Limpar filtros
e mostrar todos** e navegue por **Próxima página**. **Selecionar canais desta
busca** marca todos os resultados compatíveis, inclusive outras páginas. A
autorização só muda quando você publica com sua senha; navegar não libera canais.

## 2. No Edge: buscar a lista

1. Abra **Canais**, onde aparece **Canais do Kore Play**.
2. Na primeira vez, confirme o endereço HTTPS da central e o IP privado dela dentro do túnel WireGuard. O domínio deve corresponder ao certificado HTTPS válido da central. O IP precisa pertencer às rotas já autorizadas no WireGuard.
3. Informe a senha administrativa **deste Edge** e clique em **Salvar central**.
4. Informe novamente a senha, confirme a consulta e clique em **Buscar canais**.
5. Confira o nome do parceiro, a quantidade de canais e a última busca. Com atualização automática habilitada, a lista é renovada a cada minuto.
6. Use **Testar sinal** em um canal piloto. A tela informa se uma amostra válida foi recebida e o horário da verificação.

## 3. No Edge: assistir à prévia de um canal

1. Na lista de canais autorizados, clique em **Play** na linha do canal/protocolo.
2. Um mini player abre no canto inferior direito, com vídeo, volume e tela cheia
   nos controles do navegador. O áudio inicia silenciado: ative-o no vídeo.
3. Confira o estado **Reproduzindo**. **Carregando** significa que ainda não há
   confirmação de reprodução. Falhas mostram uma orientação, sem exibir a origem.
4. Clique no Play de outro canal para trocar. Apenas uma prévia fica ativa.
5. Use **Parar e fechar**, o **X** ou **Esc** para encerrar a transmissão.

A prévia dura até cinco minutos. Pausar encerra sua conexão; use **Reproduzir
novamente** ou Play na linha para retomar. Um fluxo sem avanço por 20 segundos
encerra com mensagem de erro. Ao sair da página, fechar, trocar de canal ou perder
a autorização, o player libera sua conexão e o buffer.

HLS usa suporte nativo quando disponível ou HLS.js; HTTP MPEG-TS usa mpegts.js
com MediaSource. As bibliotecas são servidas pelo próprio Edge. Áudio e vídeo
precisam de codecs compatíveis com o navegador; não há transcodificação automática.
Uma amostra TS válida ou playlist HLS acessível não garante que o navegador
conseguirá decodificar o conteúdo. O player ajuda a fazer essa conferência visual.

O vídeo usa sua sessão autenticada do Edge e as permissões do parceiro. O player
não divulga a URL original nem fornece um link público para reprodução externa.

## Área de trabalho e telas pequenas

As telas de canais do Edge e parceiros do Kore Play ocupam a largura disponível
ao lado do menu. Mensagens de teste, nomes longos e caminhos de mídia quebram
linha dentro das células. Em telas pequenas, cada canal aparece como um cartão
com seus campos e ações, evitando uma tabela larga que esconda os controles.

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
