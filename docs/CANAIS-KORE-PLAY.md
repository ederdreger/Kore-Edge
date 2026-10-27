# Liberar e buscar canais por parceiro

Para cadastrar diretamente uma saída Flussonic/Kore Play pelo link, use agora
**Canais → Adicionar canal** no Edge. MPEG-TS, HLS, UDP e SRT têm detecção
sem importar catálogo nem repetir a senha administrativa.
[Passo a passo do cadastro manual](modules/20-manual-channels.md).
O procedimento abaixo continua sendo a alternativa de catálogo automático.

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

## 2. No Edge: receber automaticamente

1. Abra **Canais → Sincronizar com matriz**.
2. Na primeira vez, confirme HTTPS e IP privado da central, conforme as rotas WireGuard já autorizadas. Use **Salvar central** e **Buscar canais** conforme o formulário.
3. Mantenha a atualização automática do catálogo habilitada. Sem renovação, a permissão expira.
4. Os canais autorizados iniciam pelo agente, sem depender de manter o navegador aberto. Até 16 ficam ativos; os demais aguardam vaga.
5. Na linha do canal, um único botão alterna **Iniciar/Parar**, sem pedir outra senha ou abrir confirmação. Parar persiste a pausa.
6. Para cadastrar pelo link, use **Adicionar canal**. Um novo canal habilitado inicia ao salvar. HTTP MPEG-TS, HLS, UDP MPEG-TS e SRT caller são aceitos; para HTTP sem extensão, selecione o formato da matriz.

## 3. Assistir e acompanhar quedas

Selecione o canal e use **Reproduzir**. O mini player compartilha a recepção
contínua; não inicia outra origem sob demanda. Áudio começa silenciado.
Pausar, fechar ou trocar de canal encerra apenas o player. **Parar canal**
encerra recepção e saídas.

Não há limite de cinco minutos no player contínuo. Travamentos transitórios
acionam reconexão automática; quedas da origem acionam recuperação no agente,
com espera progressiva de até 30 segundos. Em **Qualidade**, veja interrupções,
última falha e próxima tentativa. Acesso revogado e sessão expirada continuam
bloqueados. Codecs incompatíveis exigem correção na origem, sem transcodificar.

## 4. Links TS/HLS e destino UDP

Na aba **Saídas**, os links MPEG-TS e HLS aparecem automaticamente. Copie-os
para o XUI One ou outro receptor. **Lista TS/HLS** exporta os canais filtrados.
Não há botão de gerar ou revogar, nem prazo de expiração.

O endereço permanece o mesmo ao reiniciar o Edge, alterar origem ou trocar senha
administrativa. Excluir o canal invalida o endereço antigo. Não compartilhe os
links em logs ou tickets: eles incluem uma chave de reprodução exclusiva do canal.
O receptor precisa confiar no certificado HTTPS do Edge. Os endereços antigos
`/api/v1/live/...` continuam internos ao painel e exigem cookie. Substitua links
antigos com prazo pelos novos endereços permanentes exibidos automaticamente.

HLS é preparado para novos canais quando a matriz permite a saída. Precisa de
FFmpeg/ffprobe e H.264/AAC verificados. O painel informa preparação ou
incompatibilidade; não anuncia HLS como pronto antes de existirem segmentos.

Use **Configurar destino UDP** ou a engrenagem para confirmar interface, IP de
origem, grupo multicast privado e porta. Nada escolhe a rede automaticamente.
Salvar em canal ativo reinicia apenas aquele canal, com breve interrupção.
A matriz precisa liberar UDP/HLS separadamente para os canais importados.

## 5. Organização

**Lista** usa zebra e cabeçalho fixo. **Mini cards** oferece visão compacta.
Busca, categoria, protocolo e status filtram ambos. A instalação permanece
sem canais de demonstração. **Testar sinal** e o atalho duplicado
**Recepção e saídas** foram removidos da tela principal.

Se não aparecerem canais, confira parceiro, permissões publicadas, certificado,
WireGuard e atualização do catálogo. Um túnel online não libera canais por si só.
Nenhum protocolo garante ausência de cortes se a origem ou o link falharem.
Consulte [operação UDP/HLS/SRT](DISTRIBUICAO-AO-VIVO.md) e
[cadastro manual](modules/20-manual-channels.md).
