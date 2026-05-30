package main

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeConfig_RequiresPostgresDSN(t *testing.T) {
	_, err := normalizeConfig(config{
		BaseURL:     "http://localhost:8080",
		Duration:    time.Second,
		Concurrency: 1,
		Interval:    time.Second,
		Timeout:     time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "Postgres") {
		t.Fatalf("want Postgres guard error, got %v", err)
	}
}

func TestNormalizeConfig_DefaultsMetricsURL(t *testing.T) {
	cfg, err := normalizeConfig(config{
		BaseURL:     "http://localhost:8080/",
		DatabaseURL: "postgres://midas:midas@localhost/midas?sslmode=disable",
		Duration:    time.Second,
		Concurrency: 1,
		Interval:    time.Second,
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("normalizeConfig: %v", err)
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Fatalf("BaseURL trimmed: got %q", cfg.BaseURL)
	}
	if cfg.MetricsURL != "http://localhost:8080/metrics" {
		t.Fatalf("MetricsURL: got %q", cfg.MetricsURL)
	}
}

func TestParsePrometheusSamples_FiltersRuntimeSignals(t *testing.T) {
	body := `
# HELP midas_database_pool_open_connections pool
midas_database_pool_open_connections{database="postgres"} 5
midas_store_transaction_stage_duration_seconds_sum{operation="evaluation",stage="commit",result="success"} 1.25
go_goroutines 12
`
	got := parsePrometheusSamples(body, []string{
		"midas_store_transaction_stage_duration_seconds",
		"midas_database_pool_",
	})
	if got[`midas_database_pool_open_connections{database="postgres"}`] != 5 {
		t.Fatalf("pool sample missing: %#v", got)
	}
	if got[`midas_store_transaction_stage_duration_seconds_sum{operation="evaluation",stage="commit",result="success"}`] != 1.25 {
		t.Fatalf("stage sample missing: %#v", got)
	}
	if _, ok := got["go_goroutines"]; ok {
		t.Fatalf("unexpected unrelated sample: %#v", got)
	}
}

func TestSummarizeLatency_IncludesP999AndConfidence(t *testing.T) {
	got := summarize([]time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}, 2)
	if got.Count != 5 || got.P50 != 30*time.Millisecond || got.P95 != 40*time.Millisecond || got.P99 != 40*time.Millisecond || got.P999 != 40*time.Millisecond || got.Max != 50*time.Millisecond || got.Errors != 2 {
		t.Fatalf("unexpected summary: %#v", got)
	}
	if !strings.HasPrefix(got.P999Confidence, "low:") {
		t.Fatalf("unexpected p999 confidence: %q", got.P999Confidence)
	}
}

func TestP999ConfidenceThresholds(t *testing.T) {
	for _, tc := range []struct {
		samples int
		prefix  string
	}{
		{samples: 999, prefix: "low:"},
		{samples: 1000, prefix: "directional:"},
		{samples: 9999, prefix: "directional:"},
		{samples: 10000, prefix: "useful:"},
	} {
		if got := p999Confidence(tc.samples); !strings.HasPrefix(got, tc.prefix) {
			t.Fatalf("p999Confidence(%d)=%q, want prefix %q", tc.samples, got, tc.prefix)
		}
	}
}

func TestBuildPayload_UsesGovernedFixtureAndUniqueRequestID(t *testing.T) {
	body := string(buildPayload("req-test-1"))
	for _, want := range []string{
		`"surface_id":"surf-d38i-load-1"`,
		`"process_id":"proc-d38i-load-1"`,
		`"agent_id":"agent-d38i-load-1"`,
		`"request_id":"req-test-1"`,
		`"request_source":"d38i-loadtest"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("payload missing %s: %s", want, body)
		}
	}
}
