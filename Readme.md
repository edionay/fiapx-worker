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
| `METRICS_ADDR` | `:9091` | Endereço do servidor HTTP que expõe as métricas. |

## Métricas (Prometheus)

O worker sobe um servidor HTTP que expõe métricas no formato do Prometheus em `/metrics`
(porta configurável por `METRICS_ADDR`):

```shell
curl http://localhost:9091/metrics
```

Além das métricas padrão de runtime do Go (goroutines, memória e coletor de lixo), o worker
publica `videos_processados_total` com o rótulo `status` (`CONCLUIDO` ou `ERRO`) e
`video_processamento_duracao_segundos`, um histograma da duração do processamento. O scrape e
os dashboards ficam no [`fiapx-infra`](https://github.com/edionay/fiapx-infra).

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
