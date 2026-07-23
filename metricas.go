package main

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	videosProcessados = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "videos_processados_total",
		Help: "Total de vídeos processados pelo worker por status",
	}, []string{"status"})

	duracaoProcessamento = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "video_processamento_duracao_segundos",
		Help:    "Duração do processamento de vídeo em segundos",
		Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60, 120},
	})
)

func iniciarServidorMetricas(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Printf("métricas disponíveis em %s/metrics", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("servidor de métricas encerrado: %v", err)
		}
	}()
}
