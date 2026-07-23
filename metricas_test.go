package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricasExpoeRuntimeGo(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d", rec.Code)
	}
	if corpo := rec.Body.String(); !strings.Contains(corpo, "go_goroutines") {
		t.Fatalf("métricas de runtime do Go ausentes")
	}
}

func TestContadorVideosProcessados(t *testing.T) {
	antes := testutil.ToFloat64(videosProcessados.WithLabelValues("CONCLUIDO"))
	videosProcessados.WithLabelValues("CONCLUIDO").Inc()
	depois := testutil.ToFloat64(videosProcessados.WithLabelValues("CONCLUIDO"))

	if depois-antes != 1 {
		t.Fatalf("esperava incremento de 1, veio %v", depois-antes)
	}
}
