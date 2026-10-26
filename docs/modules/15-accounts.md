# Login, usuários e instalação limpa

Escopo: autenticação do painel e identidades locais. Não habilita transporte,
VPN ou matriz e não substitui a homologação de IPTV real da Fase 14.

## Comportamento

- Login responsivo com fundo local, animação CSS e respeito a movimento reduzido.
- Sem usuário, senha, canal ou contador de exemplo. O operador escolhe o admin;
  a atualização importa somente seu hash bcrypt, preservando o acesso existente.
- Administrador cria até 64 contas, altera senha/função e desativa contas. Leitura
  consulta o painel e altera somente a própria senha. Nome de usuário é imutável.
- Alterações exigem a senha atual do responsável; último admin ativo é protegido.
- Senhas novas: 12 a 72 bytes, bcrypt custo 12. Importação: bcrypt custo 10–14.
- Alterar a conta revoga suas sessões. Logout revoga a sessão atual. Reiniciar o
  core encerra todas as sessões, inclusive as marcadas com Lembrar de mim.
- Sessão: 8h absolutas/30min inativas; lembrar: 7 dias absolutos/24h inativas.
- Recuperação orienta outro administrador ou ferramenta local; não envia e-mail.

## Configuração e armazenamento

```yaml
version: 1
mode: production
security:
  mode: local_readonly
http:
  listen: "127.0.0.1:18080" # exemplo; detectar e confirmar no destino
accounts:
  enabled: true
  directory: /var/lib/kore-edge/users
  public_origin: "https://painel.exemplo.com:8443" # confirmar host e porta
```

O perfil production rejeita stream engine, HLS, provisionamento/integração mock e
multicast sintético. O instalador não configura catálogo nem inicia tráfego. Dados
locais reais continuam disponíveis; campos sem fonte ficam indisponíveis. Não há
modo de demonstração: ?demo=1 não altera os dados.

Diretório 0700 e arquivos 0600 fora do checkout. users.json contém identidades e
hashes; sessões ficam em memória, indexadas por SHA-256 do token. Gravação atômica
com sync. Lock do sistema operacional impede escritores concorrentes e libera em
crash. Lock de auditoria abandonado é recuperado somente depois desse lock.

Auditoria privada de alterações usa IDs opacos, sem nomes, senha, hash ou cookie,
com limite de 8 MiB. Escritas falham com auditoria indisponível/cheia: arquivar com
core parado conforme procedimento de segurança. Backups contêm hashes: manter
privados e nunca publicar em Git, logs ou chamados.

## HTTP e segurança

POST /api/v1/auth/login; GET /api/v1/auth/me; POST /api/v1/auth/logout;
GET/POST /api/v1/users; PATCH /api/v1/users/{id}. Respostas não incluem hashes.
Cookie __Host-kore_session: Secure, HttpOnly, SameSite=Strict, Path=/, sem Domain.
CSRF fica em memória. Mutações exigem Origin HTTPS configurada, JSON estrito,
CSRF após login e corpo até 8192 bytes. Quatro mutações concorrentes, 256 sessões,
30 tentativas/minuto por instância e 5 falhas/nome normalizado. Erros genéricos.

Somente HTML, assets, healthz e readyz são públicos. Backend apenas em loopback.
Nginx preserva Origin e remove Authorization/cabeçalhos de identidade encaminhada.
Sessão não autoriza escritas nas APIs operacionais: o guard original permanece.
Accounts exige local_readonly atrás do gateway, não se mistura com listener mTLS.

## Operação explícita pelo administrador

update-panel.sh --plan: sem mudanças. --apply exige root, terminal, unidades e
proxy reconhecidos e confirmação dos endereços. Compila, testa, vet e race como
nobody antes da parada. Sem APT. Backup de YAML, Nginx e destino do symlink.
Importa admin somente se não houver base, preserva certificado e dados anteriores,
e substitui configuração operacional por core limpo. Valida prontidão, HTTPS com
certificado explicitamente confiado e 401 sem sessão. Falha após parada restaura
configuração/release e tenta iniciar somente os serviços Kore Edge anteriores.

reset-admin.sh --apply solicita usuário existente/senha oculta, cria backup e para
somente core. edge-users reset-htpasswd altera somente admin ativo existente com
lock exclusivo. panel-access.sh encaminha à recuperação nova ao detectar a base.
Em uso normal, use o menu Usuários.

## Aceite

Testes: importação sem sobrescrita, persistência, lock, expiração, rate limit,
CSRF/Origin, cookies, logout, papéis, senha atual, último admin, revogação e recovery;
sem segredo nas respostas/auditoria. Frontend: login, senha visível, dashboard
bloqueado antes da sessão e ausência de demonstração. CI Ubuntu: Nginx extraído,
TLS loopback e credenciais sintéticas, sem instalar serviços. Revisão visual no
navegador e aceite operacional no destino ainda cabem ao operador; equivalência
pixel a pixel com a referência não é presumida.

Referências: [sessões OWASP](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html),
[autenticação OWASP](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html).
