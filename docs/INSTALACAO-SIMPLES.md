# Instalação e atualização com perguntas

## Sistema já instalado e acessível

Na pasta do projeto, execute uma linha de cada vez:

```sh
git pull --ff-only
bash scripts/update-panel.sh --plan
sudo bash scripts/update-panel.sh --apply
```

1. Leia o plano. Sem APT, firewall, rede ou VPN.
2. Confira host/IP e portas detectados. Digite ATUALIZAR somente se corretos e
   puder haver breve interrupção do painel.
3. Aguarde compilação e testes. Os serviços só param após essas verificações.
4. O script mostra o backup privado e reinicia somente core e gateway Kore Edge.
5. Abra janela privativa no HTTPS habitual. Confira a impressão digital do
   certificado local. Use o mesmo usuário e senha anteriores.
6. No menu **Usuários**, crie contas ou escolha **Editar** para alterar senha,
   inclusive do administrador. Confirme com sua senha atual. Alterar sua própria
   conta exige novo login.

Sem dados de demonstração. A atualização desabilita catálogo, streams e integrações
de laboratório na configuração; arquivos anteriores são preservados. A base de
usuários nunca é sobrescrita. Gateway personalizado é recusado para revisão manual.
Não execute o instalador novo sobre uma instalação existente.

Se a validação falhar, o script restaura configuração e release anteriores.
Backup: edge.yaml, nginx.conf e release (caminho anterior do symlink).
Rollback manual: pare somente os dois serviços Kore Edge, restaure os dois arquivos
nos destinos originais e aponte /opt/kore-edge/current ao caminho registrado em
release. Inicie esses serviços e valide readyz em loopback e HTTPS. Não remova a
base de usuários nem compartilhe backups privados.

## Instalação nova: Ubuntu Server 24.04

```sh
git clone https://github.com/ederdreger/Kore-Edge.git
cd Kore-Edge
bash scripts/setup.sh
```

Cinco etapas:

1. **Instalação limpa:** confirme a preparação, sem canais ou senhas de exemplo.
2. **Pacotes:** completo inclui FFmpeg/WireGuard tools; core inclui painel/API.
3. **Acesso:** escolha/digite domínio ou IP da VPS, porta HTTPS e usuário admin.
   IP privado funciona via LAN/VPN; internet depende de NAT/DNS/provedor.
4. **Início:** escolha iniciar os novos serviços ao terminar ou somente instalar.
5. **Resumo:** confira endereços e escolha instalar, simular ou cancelar.

A senha do painel é solicitada duas vezes, oculta: 12 a 72 bytes. Sudo pode pedir
separadamente sua senha do Ubuntu. Nada precisa ser enviado ao suporte. O sistema
instala pacotes ausentes, testa, compila e cria a conta escolhida. Não habilita
início no boot automaticamente.

Abra o HTTPS exibido. O certificado inicial é autoassinado: confira a impressão
digital. O login aparece na página, sem janela Basic Auth. Instalação limpa não
substitui homologação de IPTV real: módulos sintéticos/mock ficam desabilitados
no perfil production. Não há conta ou senha padrão.

## Recuperar o único administrador

```sh
bash scripts/reset-admin.sh --plan
sudo bash scripts/reset-admin.sh --apply
```

Informe o admin existente, confirme a breve interrupção e digite a nova senha
duas vezes. O script salva backup privado, para somente core, troca o hash e
inicia o core. Depois entre no navegador. Contas normais pedem redefinição a outro
administrador pelo menu Usuários.

Em instalações antigas com Basic Auth, panel-access.sh mantém a recuperação antiga;
após migração encaminha à ferramenta nova.
