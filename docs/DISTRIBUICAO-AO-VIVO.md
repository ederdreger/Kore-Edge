# Canais, SRT e saídas: por onde começar

## 1. Liberar no Kore Play

1. Confirme WireGuard online nos dois painéis.
2. Abra **Canais → Parceiros Kore Edge** e selecione o parceiro.
3. Marque os canais. Para cada canal, escolha o protocolo de entrada e marque
   separadamente as saídas locais permitidas: **UDP** e/ou **HLS**.
4. Publique com a senha administrativa do Kore Play. Permissões antigas não
   recebem saídas automaticamente.

### Se escolher SRT

Na mesma página, abra **Transporte SRT para os Edges**. Confira o IP privado da
matriz e a proposta **20000/UDP**, com latência inicial de 200 ms para revisão.
**Salvar preparação SRT** não abre o listener. A porta e a latência podem ser
alteradas antes da ativação. Não use o endpoint público do WireGuard como porta SRT.

Somente depois da revisão operacional, confirme **Iniciar listener SRT**.
O painel só confirma que está escutando após resposta real do auxiliar. Selecione
SRT nos canais do parceiro e publique. O Edge inicia a conexão privada e a
matriz envia a mídia por ela. Chaves de transporte são derivadas internamente;
não é necessário copiar senha SRT. Reiniciar o Kore Play deixa o listener parado
até nova confirmação no painel.

## 2. Receber no Edge

1. Abra **Canais → Sincronizar com matriz** e use **Buscar canais**.
   Confira a opção **Atualizar automaticamente a cada minuto** para operação
   contínua: o catálogo assinado expira em cinco minutos sem renovação.
2. Canais autorizados iniciam automaticamente, até 16 simultâneos; os demais aguardam vaga.
3. Use a engrenagem para ajustar protocolo e saídas. Salvar um canal ativo reinicia somente ele.
4. O controle **Iniciar/Parar** executa diretamente com sua sessão administrativa.
5. Aguarde **Recebendo** e bitrate medido; depois use **Reproduzir canal**.

A recepção permanece aberta com o player fechado. O player contínuo não tem
limite de cinco minutos e reconecta em falhas transitórias. A origem é monitorada
por ausência de pacotes; o agente reconecta com backoff de até 30 segundos.
Parar persiste pausa manual. Revogação e expiração continuam bloqueando o acesso.
Veja o [passo a passo atual](CANAIS-KORE-PLAY.md).

## 3. Configurar as saídas

**Multicast UDP** e **HTTP / HLS** listam todos os canais recebidos, inclusive os
parados e aqueles sem permissão da matriz. **Adicionar canal à saída** abre a
configuração do canal selecionado. Sem permissão, solicite a liberação no Kore Play.

Para UDP, selecione explicitamente interface e seu IP, grupo privado 239.x.x.x e
porta. Todos começam vazios. Use **Verificar destino sem enviar**; isso apenas
consulta o inventário. TTL é 1. Salve, revise e inicie a recepção quando autorizado.
O painel não ativa NIC, cria rota ou modifica firewall. Erro de interface/permissão
é apresentado sem conceder privilégios ao serviço automaticamente.

Para HLS, marque **Preparar saída HLS**. A recepção mede uma amostra e verifica os
primeiros fluxos selecionados de vídeo e áudio. Apenas H.264/AAC são aceitos para
esta saída, sem transcodificação. **Preparando segmentos** não significa pronto;
o estado só passa a ativo com playlist real recente. A reprodução HLS local exige
sessão autenticada do Edge; não é uma URL pública anônima para dispositivos.

Para mudar saídas ou protocolo de um canal ativo: **Parar → Editar → Salvar →
Iniciar**. A retomada após reiniciar o agente é opcional e precisa ser marcada
explicitamente. Parar o canal remove essa autorização de retomada.

## Antes de ativar em servidor

Não há ativação automática nesta entrega. A revisão deve conferir identidade do
host, backup/release atual, espaço, ferramentas/codecs, túnel e porta disponível.
Exemplos de pré-verificações somente de leitura, com os valores confirmados pelo
operador:

```sh
command -v ffmpeg ffprobe
ffmpeg -version
df -h
ss -lun
# Substitua INTERFACE_CONFIRMADA; não publique a saída com dados de terceiros.
sudo wg show INTERFACE_CONFIRMADA latest-handshakes
```

Validação posterior: listener confirmado, catálogo sincronizado, primeiro pacote,
bitrate e contadores TS reais, reprodução com um canal piloto e saídas somente
nos destinos revisados. Valide sob carga antes de ampliar a seleção. SRT melhora
a recuperação de perdas dentro da janela configurada, mas não corrige origem
defeituosa, banda insuficiente, codecs incompatíveis ou GOP longo.

Rollback de mídia: **Parar** os novos canais e o listener SRT no Kore Play. Isso
encerra sockets e subprocessos próprios, sem remover WireGuard. Para rollback
de código, use o backup/release registrado pelo atualizador de cada aplicação,
com os serviços parados e preservando contas, identidade e dados privados.
