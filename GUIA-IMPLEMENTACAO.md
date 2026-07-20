# Guia de Implementação — Sistema de Processamento de Vídeos (FIAP X)

> **Como usar este documento:** ele é autocontido e prescritivo. Para gastar menos tokens,
> **cole no modelo executor apenas UM incremento por vez** (as seções "INCREMENTO N"), junto
> com a seção "0. CONTEXTO FIXO" no início da conversa. Cada incremento diz exatamente o que
> criar e como validar antes de seguir.

---

## 0. CONTEXTO FIXO (colar sempre no começo)

**Objetivo:** evoluir um protótipo monolítico em Go (upload de vídeo → extrai frames com ffmpeg
→ zip → download) para uma arquitetura de microsserviços, para um trabalho de faculdade.
Precisa ser **didático, legível e defensável em vídeo de até 10 min**. Sem over-engineering.

**Projeto base:** existe em `/Users/edionay/Dev/projeto-fiapx` (um `main.go` Go/Gin). A única
lógica a reaproveitar é o processamento ffmpeg+zip (funções `processVideo`, `createZipFile`,
`addFileToZip`, `isValidVideoFile` do `main.go`).

**Regras de código (valem para tudo):**
- Código novo: **Node + TypeScript** (api e notificacao) e **React + Tailwind** (frontend).
  Reaproveitar em **Go** só o worker.
- **Nomes em português** (pacotes, diretórios, tipos, variáveis): `usuario`, `video`,
  `repositorio`, `casoDeUso`. Manter inglês só no que é idioma da linguagem (ex.: `Handler`, `main`).
- **Sem comentários no código** — nomes descritivos e funções curtas resolvem.
- **Clean Architecture** por serviço: camadas `dominio` → `aplicacao` → `adaptadores`
  (dependências apontam para dentro). Interfaces ("portas") só onde há troca real: repositório,
  fila, mailer.
- Serviços **stateless**, config por **variáveis de ambiente** (preparado para Kubernetes).
- **Database-per-service:** nenhum serviço compartilha banco. Apenas a **api** tem banco (Postgres);
  **worker** e **notificacao** são stateless (não usam banco). O worker reporta o resultado por
  evento e a **api** atualiza o próprio banco.

**Arquitetura (fluxo):**
```
Frontend (React) --HTTP--> api (Node/TS + TSOA/Swagger)
   api --publica--> RabbitMQ [fila: videos.processar] --> worker (Go, stateless): ffmpeg+zip
   worker --publica--> RabbitMQ [fila: videos.resultado] --> api (atualiza o PRÓPRIO banco) ; cache em Redis
   api, se resultado = ERRO --publica--> RabbitMQ [fila: notificacoes] --> notificacao (Node/TS) --> Mailhog (e-mail)
   Prometheus faz scrape de /metrics dos serviços ; Grafana mostra dashboard
```
Comunicação entre serviços é **sempre por mensageria (RabbitMQ)**, nunca HTTP síncrono.
Só frontend↔api é HTTP. Isso atende "não perder requisição em picos" e permite escalar workers.
O fluxo por eventos é uma **SAGA por coreografia** (o caminho de erro = passo compensatório).

**Repositórios (separados, clonados lado a lado em `/Users/edionay/Dev/`):**
- `fiapx-api` (Node/TS/TSOA) — auth, upload, status, download, Swagger.
- `fiapx-worker` (Go) — consome fila, ffmpeg+zip, atualiza status, publica erro.
- `fiapx-notificacao` (Node/TS) — consome fila de erros, envia e-mail.
- `fiapx-frontend` (React+Tailwind) — login, upload, status, download.
- `fiapx-infra` — docker-compose, config Prometheus/Grafana, documentação (DDD, BDD, ADR, LGPD).
  (O `init.sql` NÃO fica aqui; pertence ao `fiapx-api` e é apenas montado pelo compose.)

**Estrutura de pastas (repos irmãos numa pasta de trabalho comum) e onde rodar:**
```
~/Dev/                        <- pasta de trabalho comum (NÃO é um repositório)
├── fiapx-api/                <- repo (tem Dockerfile e db/init.sql)
├── fiapx-worker/             <- repo (tem Dockerfile)
├── fiapx-notificacao/        <- repo (tem Dockerfile)
├── fiapx-frontend/           <- repo (tem Dockerfile)
└── fiapx-infra/              <- repo  ▶ RODE DAQUI: `docker compose up --build`
    └── docker-compose.yml    (usa build: ../fiapx-api, ../fiapx-worker, etc.)
```
Os serviços apontam para os repos irmãos por caminho relativo (`build: ../fiapx-api`), então todos
precisam estar clonados na mesma pasta e com esses nomes.

**Fluxo de trabalho (fatias verticais, infra sob demanda):**
- O sistema é construído **um serviço por vez** (fatia vertical). A infra (Postgres, RabbitMQ,
  Redis, Mailhog) entra no `docker-compose.yml` **no incremento em que um serviço primeiro precisa
  dela**, nunca antes. O `docker-compose.yml` começa inexistente e cresce a cada fatia.
- Ordem dentro de cada incremento:
  1. Escrever o **código + testes unitários com fakes** (não precisa de infra no ar).
  2. Subir no compose **a infra que aquele serviço usa** (`docker compose up -d <infra>`).
  3. Rodar o serviço **localmente** (`npm run dev` / `go run`) apontando para a infra do Docker e
     validar de ponta a ponta.
  4. **Ao estabilizar**, criar o `Dockerfile` do serviço e adicionar a entrada dele no compose
     (`build: ../fiapx-xxx`), para o serviço também rodar containerizado.
- Regra prática: **infra sempre no Docker**; o serviço em desenvolvimento roda **local** (recarga
  rápida) e é containerizado quando amadurece.

**Padrão de todo repositório novo (no bootstrap):** criar `.githooks/pre-commit` (bloqueia commit
direto em `main`/`master`) e `scripts/ativar-hooks.sh` (roda `git config core.hooksPath .githooks`),
ativar o hook, e documentar no README que, após clonar, é preciso rodar `sh scripts/ativar-hooks.sh`
uma vez. Trabalho sempre em branch, integrado à `main` por pull request.

**Infra (containers, adicionados sob demanda):** postgres, redis, rabbitmq (com management UI),
mailhog e, por último, prometheus/grafana. Armazenamento de vídeos/zip em **volume Docker
compartilhado** (`/data/uploads`, `/data/outputs`), adicionado junto com o RabbitMQ.

**Contratos de fila (JSON), cada serviço define os próprios tipos:**
- Fila `videos.processar` (api→worker): `{ "videoId": string, "caminhoEntrada": string }`
- Fila `videos.resultado` (worker→api): `{ "videoId": string, "status": "PROCESSANDO"|"CONCLUIDO"|"ERRO", "caminhoZip"?: string, "mensagemErro"?: string }`
- Fila `notificacoes` (api→notificacao): `{ "videoId": string, "emailUsuario": string, "mensagemErro": string }`

**Modelo de dados (Postgres — pertence à api; ninguém mais acessa):**
- `usuarios(id, email UNIQUE, senha_hash, criado_em)`
- `videos(id, usuario_id FK, nome_original, status, caminho_zip, mensagem_erro, criado_em, atualizado_em)`
- `status ∈ {PENDENTE, PROCESSANDO, CONCLUIDO, ERRO}`
- O schema (`init.sql`) vive no repo **`fiapx-api/db/`**; o compose do `fiapx-infra` apenas o monta.

**Portas/URLs padrão:** api `8080` (Swagger em `/docs`), frontend `3000`, rabbitmq `5672` (UI `15672`),
postgres `5432`, redis `6379`, mailhog SMTP `1025` (UI `8025`), prometheus `9090`, grafana `3001`.

---

## 1. ORDEM DE EXECUÇÃO (incrementos)

Faça na ordem (fatias verticais). A coluna "Infra" mostra o que entra no docker-compose naquele
incremento. Só avance quando a validação do atual passar.

0. Design/metodologia (docs no `fiapx-infra`). **[concluído]**
1. `fiapx-api`: autenticação (registro/login, JWT + bcrypt) + Swagger + `db/init.sql`. → **Postgres**
2. `fiapx-api`: upload de vídeo + publicação em `videos.processar`. → **RabbitMQ + volume**
3. `fiapx-api`: listagem de status + download do zip. → **Redis**
4. `fiapx-worker` (Go, stateless): consome `videos.processar`, ffmpeg+zip, publica `videos.resultado`.
   → (usa RabbitMQ e volume existentes)
5. `fiapx-api`: consome `videos.resultado` (atualiza banco) + em ERRO publica `notificacoes`. → (nada novo)
6. `fiapx-notificacao` (Node/TS): consome `notificacoes`, envia e-mail. → **Mailhog**
7. `fiapx-frontend` (React+Tailwind): login, upload, tabela de status, download. → (nada novo)
8. Monitoramento: `/metrics` nos serviços + dashboard. → **Prometheus + Grafana**
9. CI por repo (GitHub Actions + SonarCloud) + BDD automatizado na api + documentação final.
10. **(Opcional, só se sobrar tempo)** LGPD: expurgo automatizado dos pacotes vencidos +
    `DELETE /videos/:id` (exclusão pelo titular).

---

## INCREMENTO 0 — Design e documentação (repo `fiapx-infra`)

Criar `fiapx-infra/docs/` com:
- `linguagem-ubiqua.md` (DDD): glossário em português — Usuario, Video, status
  (PENDENTE/PROCESSANDO/CONCLUIDO/ERRO), Frame, Zip. Bounded contexts: api=identidade/upload,
  worker=processamento, notificacao=avisos.
- `bdd/*.feature` (Gherkin), pelo menos:
  - "Enviar vídeo válido" → fica PENDENTE e é enfileirado.
  - "Falha no processamento" → fica ERRO e um e-mail é enviado.
  - "Listar meus vídeos" → usuário vê só os próprios vídeos com o status.
- `adr/0001-split-de-linguagem.md`, `adr/0002-repos-separados.md`,
  `adr/0003-mensageria-vs-http.md`, `adr/0004-clean-architecture.md`,
  `adr/0005-saga-coreografia.md` — cada um no formato: Contexto / Decisão / Consequências.
- `lgpd.md`: minimização (só e-mail+senha), bcrypt, retenção/eliminação de vídeos, finalidade.
- `arquitetura.md`: o diagrama do fluxo (pode ser mermaid) + descrição dos serviços e filas.

**Validação:** os arquivos existem e descrevem a solução de forma coerente com o CONTEXTO FIXO.

---

## INCREMENTO 1 — API: autenticação + Postgres (repo `fiapx-api`)

Primeira fatia: o serviço de identidade e a primeira peça de infra (Postgres).

**Código (fiapx-api, Node/TS):** Node + TypeScript, Express, **TSOA** (rotas + OpenAPI/Swagger),
`pg`, `bcrypt`, `jsonwebtoken`, `jest`. Estrutura (Clean):
```
src/
  dominio/           usuario.ts (entidade + regra de e-mail/senha)
  aplicacao/
    portas.ts        interface RepositorioDeUsuario
    registrarUsuario.ts
    autenticarUsuario.ts
  adaptadores/
    web/             controllers TSOA (AuthController: POST /auth/registrar, POST /auth/login), middleware JWT
    postgres/        repositorioDeUsuarioPostgres.ts
  main.ts            composição (injeta implementações) + Swagger em /docs
db/init.sql          schema usuarios + videos (ver CONTEXTO FIXO), status default PENDENTE
```
Regras: senha com bcrypt, login retorna JWT (payload `usuarioId` e `email`), `JWT_SECRET` por env,
`DATABASE_URL` por env.
Testes (jest): `registrarUsuario` e `autenticarUsuario` com **fake** de `RepositorioDeUsuario`
(sem banco real).

**Infra no compose (fiapx-infra):** criar o `docker-compose.yml` (primeira vez) com o serviço
`postgres:16` (`POSTGRES_USER/PASSWORD/DB` por env), montando `../fiapx-api/db/init.sql` para criar
as tabelas no primeiro boot. Criar `.env.example`. Subir só o banco: `docker compose up -d postgres`.

**Rodar local e validar:** com o Postgres no Docker, rodar a api local (`npm run dev`) com
`DATABASE_URL` apontando para `localhost:5432`. Abrir `/docs`, registrar e logar via curl recebendo
token. `npm test` verde.

**Containerizar (ao estabilizar):** criar o `Dockerfile` da api e adicionar o serviço `api` ao
compose (`build: ../fiapx-api`, `depends_on: postgres`). Pode ficar para o fim da fatia da api.

---

## INCREMENTO 2 — API: upload + fila (repo `fiapx-api`) + RabbitMQ e volume

**Código (fiapx-api):**
- `dominio/video.ts` (entidade Video + enum de status).
- `aplicacao/portas.ts`: `RepositorioDeVideo`, `PublicadorDeFila`, `Armazenamento`.
- `aplicacao/enviarVideo.ts`: valida extensão (mp4/avi/mov/mkv/webm), grava o arquivo em
  `/data/uploads`, cria registro `videos` com status PENDENTE e `usuario_id` do token, publica
  `{videoId, caminhoEntrada}` em `videos.processar`. Retorna `202` com `videoId`.
- `adaptadores/web/VideoController` (protegido por JWT, upload multipart).
- `adaptadores/fila/publicadorRabbitMQ.ts` (lib `amqplib`),
  `adaptadores/postgres/repositorioDeVideoPostgres.ts`,
  `adaptadores/armazenamento/armazenamentoLocal.ts`.
- Env: `RABBITMQ_URL`, `PASTA_UPLOADS=/data/uploads`.
Testes: `enviarVideo` com fakes das três portas (extensão inválida, publica na fila, cria registro).

**Infra no compose:** adicionar `rabbitmq:3-management` (portas 5672 e 15672) e o **volume
compartilhado** `dados` montado em `/data`. Subir: `docker compose up -d postgres rabbitmq`.

**Rodar local e validar:** api local apontando `RABBITMQ_URL` para `localhost:5672`. Fazer upload
autenticado, retorno `202`, ver a mensagem na fila `videos.processar` (RabbitMQ UI `:15672`,
guest/guest) e o registro criado no Postgres.

---

## INCREMENTO 3 — API: status + download (repo `fiapx-api`) + Redis

**Código (fiapx-api):**
- `aplicacao/listarVideos.ts`: lista os vídeos do `usuario_id` do token (id, nome, status,
  criado_em, link de download quando CONCLUIDO). Cache em Redis (TTL curto, ex. 5s), invalidado
  em escrita.
- `aplicacao/baixarZip.ts` / endpoint de download que serve o zip de `/data/outputs` **somente se o
  vídeo pertence ao usuário** e está CONCLUIDO.
- `adaptadores/cache/cacheRedis.ts` (lib `ioredis`). Env `REDIS_URL`.
- Endpoints: `GET /videos`, `GET /videos/:id/download`.
Testes: `listarVideos` (filtra por usuário, usa cache) com fakes.

**Infra no compose:** adicionar `redis:7` (porta 6379). Subir: `docker compose up -d postgres rabbitmq redis`.

**Rodar local e validar:** `GET /videos` autenticado retorna só os vídeos do usuário. Download
bloqueado para vídeo de outro usuário ou não-CONCLUIDO.

> A esta altura a api está madura. Bom momento para **containerizá-la** (Dockerfile + serviço `api`
> no compose), se ainda não fez.

---

## INCREMENTO 4 — Worker (repo `fiapx-worker`, Go), stateless

Reaproveitar o `main.go` do projeto base. **Sem banco** (database-per-service): o worker só
processa e reporta o resultado por evento. Usa o RabbitMQ e o volume já existentes. Estrutura (Clean):
```
cmd/worker/main.go
interno/
  dominio/       video.go (status)
  aplicacao/     portas.go (PublicadorDeResultado, ProcessadorDeMidia), processarVideo.go
  adaptadores/
    fila/        consumidor (fila videos.processar) + publicador (fila videos.resultado)
    midia/       ffmpeg + zip (portado de processVideo/createZipFile/addFileToZip)
```
Fluxo do `processarVideo`: ao pegar a mensagem, publica em `videos.resultado` `{videoId, status:
PROCESSANDO}`, roda ffmpeg (`fps=1`), zipa os frames em `/data/outputs/frames_<videoId>.zip`,
**remove o vídeo de entrada** de `/data/uploads` (minimização LGPD, cumprida a finalidade o
original não é mais necessário, o projeto base já fazia isso com `os.Remove`) e publica
`{videoId, status: CONCLUIDO, caminhoZip}`. Em qualquer erro: publica `{videoId, status: ERRO,
mensagemErro}` e **mantém** o vídeo de entrada (permite reprocessar/investigar a falha). **O worker
NÃO acessa banco nem envia e-mail**, quem atualiza o banco e dispara a notificação é a api
(Incremento 5). Env: `RABBITMQ_URL`, `PASTA_ENTRADA=/data/uploads`, `PASTA_SAIDA=/data/outputs`.
Precisa de `ffmpeg` (base `golang` + `apk add ffmpeg`).
Testes (`go test`): `processarVideo` com fakes das portas (publicador e mídia fake, não roda ffmpeg
de verdade no unitário), verifica que publica PROCESSANDO e depois CONCLUIDO/ERRO.

**Containerizar:** por precisar de `ffmpeg`, o worker roda mais fácil no próprio container. Criar o
`Dockerfile` e adicionar o serviço `worker` ao compose (`build: ../fiapx-worker`, volume `dados`).

**Validação:** subir `worker` e `rabbitmq`; fazer upload pela api (Incremento 2) ou publicar
manualmente em `videos.processar`; ver o zip aparecer em `/data/outputs` e as mensagens de
resultado na fila `videos.resultado`. Forçar erro (arquivo corrompido) gera `ERRO` em `videos.resultado`.

---

## INCREMENTO 5 — API: consumidor de resultados + notificação (repo `fiapx-api`)

A api passa a **consumir** `videos.resultado` e atualizar o próprio banco. Não entra infra nova.
- `aplicacao/aplicarResultado.ts`: recebe `{videoId, status, caminhoZip?, mensagemErro?}`, atualiza
  o registro `videos`. Se `status = ERRO`, busca o `emailUsuario` (join `videos`→`usuarios`) e
  publica `{videoId, emailUsuario, mensagemErro}` em `notificacoes`.
- `adaptadores/fila/consumidorRabbitMQ.ts` (consome `videos.resultado`) e reuso do publicador para
  `notificacoes`.
- Invalidar o cache Redis do status do usuário ao aplicar o resultado.
Testes: `aplicarResultado` com fakes (atualiza status, em ERRO publica em `notificacoes`).

**Validação:** com worker + api no ar, fazer upload. O status caminha PENDENTE → PROCESSANDO →
CONCLUIDO no `GET /videos`. Forçar erro leva a ERRO e publica em `notificacoes`.

---

## INCREMENTO 6 — Notificação (repo `fiapx-notificacao`, Node/TS) + Mailhog

**Código (fiapx-notificacao):** estrutura Clean enxuta:
```
src/
  aplicacao/     portas.ts (EnviadorDeEmail), notificarErro.ts
  adaptadores/
    fila/        consumidor RabbitMQ (fila notificacoes)
    email/       enviadorSMTP.ts (lib nodemailer, aponta para Mailhog)
  main.ts
```
`notificarErro`: recebe `{videoId, emailUsuario, mensagemErro}` e envia um e-mail informando a
falha. Env: `RABBITMQ_URL`, `SMTP_HOST`, `SMTP_PORT`.
Testes: `notificarErro` com fake de `EnviadorDeEmail`.

**Infra no compose:** adicionar `mailhog` (SMTP 1025, UI 8025).

**Containerizar:** criar o `Dockerfile` e adicionar o serviço `notificacao` ao compose.

**Validação:** forçar um erro de processamento (a api publica em `notificacoes`), o e-mail aparece
na UI do Mailhog (`:8025`).

---

## INCREMENTO 7 — Frontend (repo `fiapx-frontend`, React + Tailwind)

Vite + React + TypeScript + Tailwind, seguindo boas práticas de design system (tokens de
cor/espaço, componentes reutilizáveis: `Botao`, `Campo`, `Cartao`, `Tabela`). Telas:
- Login/Registro (guarda o JWT).
- Upload de vídeo (envia para a api com o token).
- Tabela "Meus vídeos" com **auto-refresh** (polling em `GET /videos`) mostrando o status, com botão
  de download quando CONCLUIDO.
Env: `VITE_API_URL`. Servir via nginx (Dockerfile multi-stage). Adicionar o serviço `frontend` ao compose.

**Validação:** pela UI, registrar, logar, enviar vídeo, ver o status mudar até CONCLUIDO e baixar o zip.

---

## INCREMENTO 8 — Monitoramento (Prometheus + Grafana)

- Expor `/metrics` (formato Prometheus) em api, worker e notificacao (`prom-client` no Node,
  `client_golang` no Go). Métricas úteis: total de vídeos por status, duração de processamento,
  mensagens consumidas.
- Adicionar ao compose `prometheus` (porta 9090) e `grafana` (porta 3001), com
  `prometheus/prometheus.yml` (scrape dos 3 serviços) e o provisionamento do datasource e dashboard
  do Grafana.

**Validação:** Prometheus (`:9090`) mostra os targets "UP", Grafana (`:3001`) exibe o dashboard
reagindo aos uploads.

---

## INCREMENTO 9 — CI/CD + qualidade + docs finais

Em **cada** repo criar `.github/workflows/ci.yml` que instala deps, roda testes com cobertura, faz
build da imagem Docker e roda a análise **SonarCloud** (quality gate). Na api, incluir os cenários
**BDD** automatizados (Cucumber) rodando no CI. Finalizar a documentação no `fiapx-infra` (README com
o passo a passo de "como rodar tudo" e link dos repos).

**Validação:** push no GitHub dispara o CI, testes e quality gate passam, imagens buildadas.

---

## INCREMENTO 10 (OPCIONAL, só se sobrar tempo) — Eliminação de dados / LGPD (repo `fiapx-api`)

Duas funcionalidades de eliminação motivadas pela LGPD, ambas declaradas como "planejadas" na nota
de privacidade. Implementar só se houver tempo — se não houver, a nota já as descreve como pendentes.

### 10a. Expurgo automatizado (retenção)

Motivação: a política de retenção diz que o pacote de frames é guardado por **7 dias**. Sem uma
rotina que execute isso, a política existe só no papel e os arquivos se acumulam para sempre.

- `aplicacao/expurgarVencidos.ts`: encontra vídeos cujo `criado_em` é mais antigo que o prazo de
  retenção, remove o pacote `.zip` do armazenamento e limpa o `caminho_zip` do registro (mantendo o
  histórico do vídeo, sem o arquivo). Cobrir também os vídeos em `ERRO`, cujo vídeo de entrada
  permanece no volume — é a única coisa que os elimina.
- Porta `Armazenamento` ganha a operação de remover arquivo.
- Agendamento simples: rotina periódica dentro da api (ex.: a cada 1h, com `setInterval` na
  composição em `main.ts`). Não usar cron externo — mantém tudo em um só lugar e fácil de demonstrar.
- Env: `RETENCAO_DIAS=7`, `INTERVALO_EXPURGO_MINUTOS=60`.

Testes: `expurgarVencidos` com fakes (remove só os vencidos; não toca nos vídeos dentro do prazo).

**Validação:** criar um vídeo `CONCLUIDO` com data antiga (ajustar `criado_em` direto no banco),
rodar a rotina e conferir que o `.zip` sumiu do volume e o `caminho_zip` foi limpo — enquanto um
vídeo recente permanece intacto.

### 10b. Exclusão pelo titular (`DELETE /videos/:id`)

Motivação: direito de eliminação do titular — o usuário deve poder apagar os próprios vídeos.

- `aplicacao/excluirVideo.ts`: valida que o vídeo **pertence ao usuário do token**, remove os
  arquivos associados (vídeo de entrada, se ainda existir, e o pacote `.zip`) e apaga o registro.
- Endpoint `DELETE /videos/:id` no controller TSOA (protegido por JWT). Invalidar o cache Redis.
- Retornar `404` (não `403`) quando o vídeo é de outro usuário — não revela a existência do recurso.

Testes: `excluirVideo` com fakes (exclui o próprio; recusa o de outro usuário).

**Validação:** excluir um vídeo próprio some da listagem e remove os arquivos; tentar excluir o
vídeo de outro usuário retorna `404`.

Ao concluir qualquer uma das duas, atualizar `fiapx-infra/docs/lgpd.md`: mover o item de
"limitações/planejado" para os controles efetivamente implementados.

---

## 2. VERIFICAÇÃO END-TO-END (fecho do projeto)

1. Clonar os 5 repos como irmãos em `/Users/edionay/Dev/`; rodar `docker compose up --build` de
   dentro do `fiapx-infra` (ver "Estrutura de pastas" no CONTEXTO FIXO).
2. Abrir Swagger (`api/docs`) e o frontend (`:3000`); registrar, logar, enviar um vídeo → PENDENTE.
3. Acompanhar PROCESSANDO → CONCLUIDO; baixar o zip e conferir os frames.
4. Erro: enviar arquivo inválido → status ERRO + e-mail no Mailhog (`:8025`).
5. Picos/escala: `docker compose up --scale worker=3` e enviar vários vídeos em paralelo — nenhum se perde.
6. Monitoramento: Grafana (`:3001`) reagindo; targets "UP" no Prometheus (`:9090`).
7. Testes: `npm test` (api, notificacao) e `go test ./...` (worker); CI verde com SonarCloud.
