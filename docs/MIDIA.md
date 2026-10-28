# Midia: transmissoes e fontes

A pagina `#media` possui implementacao propria em React/CSS, baseada na organizacao funcional da referencia visual: abas, barra de ferramentas, tabela densa, linhas alternadas, resumo e rodape de paginacao. O fundo e azul-acinzentado claro. Nao utiliza codigo, bibliotecas ou recursos visuais do Flussonic.

## Operacao

- **+ Adicionar transmissao** usa o cadastro manual existente, com deteccao de HTTP MPEG-TS, HLS, UDP e SRT segundo as capacidades atuais.
- **Transmissoes** mostra entrada selecionada, recepcao continua, categoria, taxa, conexoes TS observadas e estados de saida. **Fontes** concentra entrada e edicao de cadastros locais. Enderecos locais sao os valores seguros retornados pela API; origens remotas e suas credenciais continuam protegidas na matriz.
- A busca abrange nome, categoria e protocolos. Os estados distinguem recepcao recente, espera/reconexao, parada e erro. Ordenacao por nome ou taxa, lista/cartoes e paginas de 25, 50 ou 100 itens nao alteram fontes.
- A caixa do cabecalho seleciona somente a pagina atual. A selecao conserva itens de outras paginas/filtros; o painel de lote informa isso. Operacoes sao sequenciais, usam a geracao retornada por cada acao e param na primeira falha, informando quantas terminaram. Nao ha rollback automatico de operacoes de lote ja concluidas.
- O interruptor inicia ou para o canal. Reconectar para e inicia somente a recepcao escolhida; se o novo inicio falhar, a mensagem informa a parada. Nenhuma confirmacao ou senha adicional e introduzida. O servidor continua validando sessao administrativa, CSRF, permissoes e revisoes.
- Reproduzir usa o mesmo ingest ativo. Detalhes e saidas abrem os links permanentes atuais, com as permissoes HLS existentes. Configuracao de destinos UDP continua no gerenciamento de canais.

Abrir a pagina executa somente leituras. Ela nao reinicia canais, aplica configuracoes de rede ou cria dados demonstrativos. As escolhas de parada manual sao mantidas.

## Limites desta entrega

Modelos, multiplexadores, VOD e placas DVB ficam ocultos, conforme solicitado. A coluna de transcodificacao descreve fluxo original sem conversao. A gravacao permanece indisponivel. Conexoes TS nao equivalem a clientes unicos nem incluem audiencia HLS ou receptores atras de um XUI. Taxa de saida sem medicao fica indisponivel; a entrada indica quando a soma e parcial.

## Criterios de aceite e verificacao

1. Navegar para Midia e alternar Transmissoes/Fontes em portugues.
2. Filtrar, ordenar e paginar mantendo dados reais e selecao limitada a pagina ao usar o cabecalho.
3. Trocar lista/cartoes mantendo os filtros e operacoes.
4. Abrir sem POST ou conexao adicional com origem; adicionar somente por acao explicita.
5. Espectador nao consegue acionar interruptor, cadastro, reconexao ou lote.
6. Lote usa uma geracao atualizada por acao; reconectar age somente no canal escolhido.
7. Dados ausentes, grava??o indisponivel e fluxo sem transcodificacao nao apresentam estatisticas simuladas.
8. Dialogos permitem Escape, foco contido e retorno ao controle anterior.

Testes automatizados usam catalogos e metricas sinteticos com a matriz simulada. Nenhuma alteracao de VPS faz parte desta implementacao de interface.
