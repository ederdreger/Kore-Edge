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
3. **Acesso ao painel:** selecione o número de um endereço local detectado.
   Digite a porta que deseja usar. O assistente verifica o formato e, quando a
   ferramenta está disponível, avisa se a porta já está ocupada.
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
O assistente não pede senha de GitHub, token da matriz ou chave WireGuard.

Serão verificados o sistema e os pacotes, e o código será compilado e testado.
Isso pode demorar. Se um teste falhar, a instalação será interrompida; não ignore
o erro nem remova os testes. Uma instalação existente não será sobrescrita.

No final, o programa mostrará o endereço local do painel. O perfil completo
instala as ferramentas, mas não cria canais, túneis ou catálogo automaticamente.
O serviço não fica habilitado para iniciar automaticamente no boot.

## 5. Abra o painel

Se escolheu iniciar o serviço, abra o endereço mostrado **no próprio servidor**.
Se está conectado por SSH, esse endereço pertence ao Ubuntu, não ao seu computador.
O painel continua restrito ao acesso local por segurança.

Para abrir no computador que usa para acessar o servidor, configure o encaminhamento
de porta no seu cliente SSH. Use a porta que você escolheu como porta de origem e
o mesmo endereço local do Ubuntu como destino. No OpenSSH, a forma é:

```text
ssh -N -L PORTA_NO_SEU_COMPUTADOR:IP_LOCAL_DO_UBUNTU:PORTA_DO_PAINEL USUARIO@SERVIDOR
```

Esses nomes representam os seus dados, não devem ser colados literalmente.
O comando é executado no seu computador. Mantenha o terminal SSH aberto e acesse
`http://localhost:PORTA_NO_SEU_COMPUTADOR` no navegador. Não abra firewall nem
troque o listener para `0.0.0.0` como atalho.

Para detalhes de pré-verificação, arquivos criados e rollback, consulte
[o guia técnico](INSTALL.md). O assistente não foi aplicado em um host de produção.
