# Midia - cadastro e pagina do canal

## Fluxo implementado

1. Em Midia, usar + para abrir `#media/new`.
2. Informar Nome do canal e URL da origem. Links reconhecidos indicam HTTP MPEG-TS, HLS, SRT ou UDP. HTTP sem formato evidente permite escolha explicita. UDP exige interface e IP local consultados no inventario e confirmados pelo operador.
3. Salvar usa a sessao atual, CSRF e revisao do cadastro. A API existente solicita a recepcao continua. Uma falha de cadastro mantem o formulario com o erro; cancelar nao grava nada.
4. Apos sucesso, abrir `#media/channel/ID/overview`. O fragmento contem somente o identificador; nenhuma URL, senha ou parametro da origem vai para o historico. O nome na lista abre a mesma pagina, e Transmissoes retorna a listagem.

## Visao geral

O tema continua azul-acinzentado claro, com descricao no topo, player a esquerda e informacoes/grafico a direita em telas grandes. Nome, titulo e descricao possuem salvamento proprio, sem parar recepcao. O player so abre por acao explicita e reutiliza o ingest existente; nao cria outra conexao com a origem. Os controles iniciam/param e reconectam somente o canal selecionado. O modo e continuo. A saida HLS so pode ser reproduzida quando autorizada e pronta.

O grafico contem apenas amostras observadas desde a abertura, limitadas a 120. Entrada e conexoes TS possuem escalas independentes. Ausencia de taxa cria lacuna, nao um valor simulado. Taxa de saida, codecs e resolucao ainda nao medidos aparecem como tais. Nao ha transcodificacao ou dados extraidos das imagens.

Entrada apresenta protocolo, endereco sanitizado e parametros UDP disponiveis. Saidas reutiliza os links permanentes TS/HLS e mostra o destino UDP ja configurado. Configuracao avancada de entrada, transcodificacao, gravacao, EPG, autenticacao por canal e sessoes sera detalhada nas proximas etapas; suas abas adicionais estao explicitamente desabilitadas. Nao ha modelos, DASH, gravacao ou incorporacao publica ficticios.

Excluir canal exige confirmacao explicita. A recepcao ativa e parada antes da exclusao; se a exclusao falhar, a mensagem informa a parada. A API existente invalida os links permanentes. Canais da matriz permanecem gerenciados nela; espectadores nao podem cadastrar, editar metadados ou excluir.

## Contrato de metadados

POST `/api/v1/manual-channels` aceita `action: metadata`, revisao atual, confirmacao, `source: {id}` e `metadata: {name,title,description}`. Somente o administrador autenticado pode executar. Nome e limitado a 128 bytes, titulo a 256 e descricao a 2048, em UTF-8 valido. O inventario sanitizado retorna os campos de apresentacao. Metadados nao recebem nem substituem URL, credenciais, protocolo ou habilitacao.

`presentation.json`, no mesmo diretorio privado das origens, possui versao e revisao proprias, permissao 0600 e substituicao atomica. A revisao publica combina as revisoes de origem e apresentacao, evitando sobrescritas antigas. O nome de apresentacao e associado ao nome-base cadastrado; uma alteracao explicita do nome no editor de origem continua prevalecendo. Alterar somente apresentacao nao reescreve `channels.json`; versoes anteriores podem ignorar o novo arquivo e continuar lendo suas origens. Nao se exportam URLs privadas ou valores de metadados para logs/auditoria.

## Aceite automatizado

- Salvar nome/URL redireciona para o identificador real retornado pela API.
- URL com credenciais sinteticas nao aparece no fragmento nem no formulario apos o redirecionamento.
- Falha de salvamento nao redireciona; cancelar nao adiciona gravacoes.
- Links diretos de entrada e saida e retorno para Transmissoes funcionam.
- UDP nao escolhe interface/IP por conta propria.
- Espectador nao acessa operacoes administrativas; sessao e CSRF continuam obrigatorios.
- Salvar descricao de um canal ativo mantem sua recepcao e geracao de distribuicao.
- Nome/titulo/descricao persistem ao reabrir, rejeitam revisoes antigas e limites invalidos, preservam credenciais e o arquivo de origens.

Esta revisao e de codigo local; nao aplica mudancas em VPS, rede, firewall ou servicos.
