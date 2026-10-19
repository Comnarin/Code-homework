package metrics

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestServe_AnswersOnlyMetricsAndStopsWithTheContext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	reg := prometheus.NewRegistry()
	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: "probe_gauge", Help: "h"})
	reg.MustRegister(g)
	ctx, cancel := context.WithCancel(context.Background())
	Serve(ctx, addr, reg)

	var body string
	for i := 0; i < 50; i++ {
		resp, err := http.Get("http://" + addr + "/metrics")
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			body = string(b)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(body, "probe_gauge") {
		t.Fatalf("metrics body = %q", body)
	}
	resp, err := http.Get("http://" + addr + "/api/products")
	if err != nil || resp.StatusCode != 404 {
		t.Fatalf("other paths must 404: err=%v resp=%v", err, resp)
	}
	resp.Body.Close()

	cancel()
	time.Sleep(200 * time.Millisecond)
	if _, err := http.Get("http://" + addr + "/metrics"); err == nil {
		t.Fatal("server should be stopped after the context ends")
	}
}
