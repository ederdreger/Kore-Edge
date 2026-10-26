# Conectar Edge e Kore Play, passo a passo

Ao abrir **WireGuard**, a tela principal mostra o estado atual. **Online** exige
resposta recente real; **Sem resposta** significa que a configuração foi aplicada
mas não houve resposta recente; **Indisponível** significa falha de consulta e não
mantém uma confirmação antiga como atual. A consulta continua a cada cinco segundos,
inclusive depois de conectar. Use **Ver detalhes da conexão** para acompanhar sem
reconfigurar e **Voltar à tela principal do WireGuard** para sair do assistente.
Quando a configuração já está aplicada, não é necessário repetir os botões.

Abra os dois painéis em abas diferentes. Use uma conta de administrador em cada um.
Cada tela indica de qual painel você precisa da senha. Não envie senhas pelo chat.

1. **Comece no Edge → WireGuard.** Se aparecer Preparar este Edge, clique e confirme.
   Clique em **Copiar identificação do Edge**.
2. **Vá ao Kore Play → Canais → WireGuard Kore Edge.** Se a central já está
   configurada, a tela começa na lista de Edges. Se não, siga Preparar central e
   Endereço da central. Confira o domínio/IP e a porta propostos antes de salvar.
3. Em **Adicionar Edge**, dê um nome e cole a identificação. Clique em Continuar.
   O endereço interno sugerido aparece para sua confirmação. Se o Edge já existe,
   escolha **Continuar com este Edge** na lista, sem cadastrá-lo novamente.
4. Se aparecer **Corrigir e continuar**, confirme a correção do endereço antigo.
   A identificação e o IP interno serão preservados.
5. Clique em **Preparar conexão da central**, confira o resumo e confirme com a
   senha do Kore Play. Aguarde a central ligar. Isso ainda não confirma comunicação.
6. Clique em **Gerar código para levar ao Edge** e copie o código. Ele vale dez minutos.
7. Volte ao **Edge → WireGuard → Já tenho o código do Kore Play**. Cole o código,
   confira o endereço do seu Kore Play e confirme com a senha do Edge.
8. Confira o resumo e clique em **Confirmar e ativar conexão**. Aguarde a resposta
   do Kore Play. A tela verifica a comunicação e mostra **Conectado ao Kore Play**
   somente quando observa resposta real recente. Isso não valida o transporte de canais.

Não confunda os códigos: **identificação** sai do Edge; **código de conexão** sai
do Kore Play. Nenhum deles é uma chave privada. Código expirado: gere outro.
“Senha não aceita”: use a senha do painel em que está agora.

As opções técnicas e o cadastro anterior ficam em **Configuração avançada**.
Instalações novas mostram sugestões, que precisam de confirmação. Conflitos de
IP, porta ou redes são verificados antes da aplicação. O assistente não abre
portas no provedor ou roteador: se a central ligar e não houver resposta, o
responsável pela rede deve conferir a porta UDP exibida no resumo.

Atualizações preservam identidades e cadastros. Desfazer uma conexão continua
exigindo confirmação. [Procedimento técnico e rollback](modules/17-wireguard-connection.md).
