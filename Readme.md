# fiapx-worker

Worker de processamento de vídeo do Sistema de Processamento de Vídeos FIAP X. É **stateless**:
não tem banco nem HTTP. Consome pedidos de uma fila do RabbitMQ, extrai os frames com **ffmpeg**
(1 por segundo), monta um `.zip` e reporta o resultado por outra fila.

> Contratos das filas e visão do sistema no repositório
> [`fiapx-infra`](https://github.com/edionay/fiapx-infra).

## Fluxo

1. Consome `videos.processar` — `{ videoId, caminhoEntrada }`.
2. Publica `videos.resultado` com `PROCESSANDO`.
3. Roda o ffmpeg sobre `caminhoEntrada` (no volume compartilhado) e gera `OUTPUTS_DIR/<videoId>.zip`.
4. Publica `videos.resultado` com `CONCLUIDO` e o `caminhoZip`, ou `ERRO` e a `mensagemErro`.

## Variáveis de ambiente

| Variável | Padrão | Descrição |
|---|---|---|
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672` | Conexão com o RabbitMQ. |
| `OUTPUTS_DIR` | `/data/outputs` | Diretório dos pacotes gerados (volume compartilhado). |

## Rodar

Requer **ffmpeg** disponível.

```shell
go run .
```

Com Docker:

```shell
docker build -t fiapx-worker .
docker run --rm \
  -e RABBITMQ_URL="amqp://fiapx:fiapx@rabbitmq:5672" \
  -v dados:/data \
  fiapx-worker
```

A stack completa (RabbitMQ, volume compartilhado e demais serviços) sobe pelo `fiapx-infra`.
