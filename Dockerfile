FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /worker .

FROM alpine:3.20
RUN apk add --no-cache ffmpeg
ENV OUTPUTS_DIR=/data/outputs
RUN mkdir -p /data/outputs
COPY --from=build /worker /usr/local/bin/worker
EXPOSE 9091
ENTRYPOINT ["worker"]
