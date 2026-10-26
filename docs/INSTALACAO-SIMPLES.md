# Instalação com perguntas, passo a passo

Use um servidor **Ubuntu Server 24.04 de laboratório** com acesso à internet.
O projeto ainda não está liberado para produção.

## 1. Abra o terminal do Ubuntu

Você pode estar no próprio servidor ou conectado a ele por SSH.
Para baixar o projeto é necessário ter o comando `git` disponível.

## 2. Baixe e abra o projeto

Na primeira vez, execute uma linha de cada vez:

```sh
git clone https://github.com/ederdreger/Kore-Edge.git
cd Kore-Edge
bash scripts/setup.sh
```

Se a pasta já existe, entre nela e atualize:

```sh
cd Kore-Edge
git pull --ff-only
bash scripts/setup.sh
```

## 3. Responda às cinco etapas

1. **Servidor de laboratório:** responda `s` somente se for um servidor de testes.
2. **O que instalar:** escolha `1` para o sistema completo com ferramentas de mídia,
   ou `2` para apenas painel e API. Enter escolhe o completo.
3. **Acesso ao painel:** digite o domínio apontado para a VPS ou o IP da VPS,
   sem `http://`, sem porta e sem caminho. Depois informe a porta HTTPS e o
   usuário para entrar no painel. Não escolha IP de loopback/localhost.
   A porta interna da API é escolhida separadamente e aparece no resumo.
   Os IPs das interfaces são detectados e classificados; você pode escolher um
   número da lista ou digitar outro domínio/IP. IP privado é aceito: o painel
   funciona na LAN/VPN; acesso pela internet requer NAT/domínio externo adequado.
   Não é feita consulta a serviços externos para descobrir IP público de NAT.
4. **Início do painel:** responda `s` para iniciar ao terminar, ou `n` para somente
   preparar a instalação. Enter significa não iniciar.
5. **Resumo:** confira os dados e escolha `1` para instalar, `2` para apenas simular
   ou `3` para cancelar. Enter apenas simula. Até essa escolha, não há alteração.

Não é necessário digitar os parâmetros `--apply`, `--lab-only` ou `--listen`.
O assistente prepara esses parâmetros com as suas respostas. A instalação real
exige um terminal interativo; colar respostas por pipe não instala nada.

## 4. Aguarde a instalação

Se aparecer uma solicitação do sudo, digite a senha do **seu usuário administrador**.
É normal não aparecerem caracteres enquanto você digita a senha.
O assistente também pede uma senha para o painel HTTPS e sua confirmação, com
entrada oculta. Use de 12 a 72 caracteres (até 72 bytes), sem dois-pontos.
Essa senha não é enviada ao GitHub nem passada como argumento de comando.
Não são solicitados token da matriz ou chave WireGuard.

Serão verificados o sistema e os pacotes, e o código será compilado e testado.
Isso pode demorar. Se um teste falhar, a instalação será interrompida; não ignore
o erro nem remova os testes. Uma instalação existente não será sobrescrita.

No final, o programa mostrará `https://DOMINIO_OU_IP:PORTA/`. O perfil completo
instala as ferramentas, mas não cria canais, túneis ou catálogo automaticamente.
O serviço não fica habilitado para iniciar automaticamente no boot.

## 5. Abra o painel

### Se uma tentativa parou durante os testes

Se apareceu `TestRejectPublicSecretPermissions: public secret accepted`, atualize
o projeto: a fixture desse teste foi corrigida para a umask privada do instalador.
Não desative o teste nem altere permissões de arquivos reais.

Se os pacotes já foram instalados e o Nginx padrão ficou mascarado/inativo pela
tentativa anterior, execute dentro da pasta Kore-Edge:

```sh
git pull --ff-only
bash scripts/setup.sh --resume-packages
```

Responda novamente às etapas. Esse modo reutiliza pacotes, sem sobrescrever sites,
configuração ou dados existentes. Se houver Nginx em execução, máscara diferente
ou uma instalação do Kore Edge já publicada, ele recusa a retomada para inspeção.
Os avisos do Ubuntu sobre reinícios adiados não são esse erro de teste.

### Se o Nginx falhou com `/var/lib/nginx/fastcgi ... Permission denied`

Os testes passaram, mas o Nginx tentou usar seu cache padrão sem permissão.
A correção configura todos os temporários no cache privado do Kore Edge.
Não apague arquivos e não aumente permissões de `/var/lib/nginx`.

Dentro da pasta Kore-Edge, atualize e confira o plano:

```sh
git pull --ff-only
bash scripts/repair-gateway.sh
```

Para aplicar o reparo no laboratório e iniciar o painel:

```sh
sudo bash scripts/repair-gateway.sh --apply --start
```

Ele exige os serviços inativos, verifica os arquivos, faz backup de `nginx.conf`,
corrige os três caminhos e valida Nginx como kore-edge. Se a validação falhar,
restaura a configuração anterior. Senha, certificado, domínio, porta e binários
são preservados. Esse reparo não reinstala pacotes nem recompila o sistema.
Com `--start`, verifica o core e inicia os dois serviços próprios; se o início
falhar, para os novos serviços e deixa a configuração corrigida para inspeção.
O endereço do painel é mostrado ao final. A instalação não fica habilitada no boot.

O reparo recupera os arquivos já publicados antes da falha. Não substitui o
bootstrap completo e não gera um novo manifesto de instalação. Futuras trocas
de release continuam exigindo o procedimento operacional revisado.

### Acesso ao painel

### Se apareceu `bind() to 0.0.0.0:443 failed (13: Permission denied)`

Esse erro não indica IP privado incompatível. A validação anterior rodava sem
a permissão necessária para a porta HTTPS privilegiada. Ela agora é executada
em uma unidade temporária como kore-edge, com CAP_NET_BIND_SERVICE, tal como o
serviço definitivo. Não é necessário mudar o IP ou trocar a porta para contornar
o problema. Atualize e repita o reparo, preservando os seus dados:

```sh
git pull --ff-only
sudo bash scripts/repair-gateway.sh --apply --start
```

O plano de preflight, backup e rollback é o mesmo do reparo de cache acima; a
validação temporária tem timeout de 20 segundos e é coletada após terminar.
Esse caminho não executa o gateway como root nem muda permissões globais do Linux.

### Acesso HTTPS

Se escolheu iniciar, abra a URL HTTPS mostrada no seu computador. Digite o usuário
e a senha cadastrados quando o navegador solicitar. Não precisa de túnel SSH.

O certificado inicial é **autoassinado para laboratório**. O navegador vai avisar
que a autoridade não é confiável. Confira a impressão digital exibida no terminal
antes de confiar nesse certificado. Não é um certificado público Let's Encrypt;
para acesso sem aviso, será necessário configurar um certificado confiável.

Se usar domínio, configure antes o DNS para apontar para a VPS. A porta HTTPS
escolhida precisa estar permitida nas regras do provedor/firewall. O instalador
não modifica essas regras. Se já houver Nginx, ele recusa sobrescrever os sites.

O acesso externo é de **leitura**. A API continua em loopback e não é exposta
diretamente. O login do gateway não concede permissões de operação de streams.

Para detalhes de pré-verificação, arquivos criados e rollback, consulte
[o guia técnico](INSTALL.md). O assistente não foi aplicado em um host de produção.
