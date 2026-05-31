package main

import (
	"os"
	"strings"
	"testing"
)

func TestHelmChart_ExposesPostgresPoolValues(t *testing.T) {
	values := readChartFile(t, "../../charts/midas/values.yaml")
	for _, want := range []string{
		"maxOpenConns: 25",
		"maxIdleConns: 5",
		`connMaxLifetime: "30m"`,
		`connMaxIdleTime: "5m"`,
	} {
		if !strings.Contains(values, want) {
			t.Fatalf("values.yaml must expose %q", want)
		}
	}
}

func TestHelmChart_RendersPostgresPoolConfigKeys(t *testing.T) {
	configMap := readChartFile(t, "../../charts/midas/templates/configmap.yaml")
	for _, want := range []string{
		"max_open_conns: {{ .Values.midas.store.maxOpenConns }}",
		"max_idle_conns: {{ .Values.midas.store.maxIdleConns }}",
		`conn_max_lifetime: {{ .Values.midas.store.connMaxLifetime | quote }}`,
		`conn_max_idle_time: {{ .Values.midas.store.connMaxIdleTime | quote }}`,
	} {
		if !strings.Contains(configMap, want) {
			t.Fatalf("configmap.yaml must render %q", want)
		}
	}
}

func TestHelmChart_ExposesDispatcherTuningValues(t *testing.T) {
	values := readChartFile(t, "../../charts/midas/values.yaml")
	for _, want := range []string{
		"batchSize: 100",
		`pollInterval: "2s"`,
		`maxBackoff: "30s"`,
	} {
		if !strings.Contains(values, want) {
			t.Fatalf("values.yaml must expose %q", want)
		}
	}
}

func TestHelmChart_RendersDispatcherTuningConfigKeys(t *testing.T) {
	configMap := readChartFile(t, "../../charts/midas/templates/configmap.yaml")
	for _, want := range []string{
		"batch_size: {{ .Values.midas.dispatcher.batchSize }}",
		`poll_interval: {{ .Values.midas.dispatcher.pollInterval | quote }}`,
		`max_backoff: {{ .Values.midas.dispatcher.maxBackoff | quote }}`,
	} {
		if !strings.Contains(configMap, want) {
			t.Fatalf("configmap.yaml must render %q", want)
		}
	}
}

func TestHelmChart_DocumentsHighWritePoolGuidance(t *testing.T) {
	readme := readChartFile(t, "../../charts/midas/README.md")
	for _, want := range []string{
		"Pool sizing for high-write runtime use",
		"replicaCount * midas.store.maxOpenConns <= postgres_max_connections - reserved_headroom",
		"default pool, concurrency 8",
		"constrained pool, concurrency 16",
		"Redis/read-model caching does not reduce governed transaction commits",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README.md must document %q", want)
		}
	}
}

func readChartFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}
