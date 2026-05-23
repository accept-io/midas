package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/postgres"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

const (
	loadBusinessServiceID = "bs-d38i-load-1"
	loadCapabilityID      = "cap-d38i-load-1"
	loadProcessID         = "proc-d38i-load-1"
	loadSurfaceID         = "surf-d38i-load-1"
	loadAgentID           = "agent-d38i-load-1"
	loadProfileID         = "prof-d38i-load-1"
	loadGrantID           = "grant-d38i-load-1"
)

type config struct {
	BaseURL     string
	MetricsURL  string
	DatabaseURL string
	Token       string
	Duration    time.Duration
	Concurrency int
	Rate        int
	Interval    time.Duration
	Timeout     time.Duration
	Seed        bool
}

type result struct {
	Duration time.Duration
	Status   int
	Err      string
}

type latencySummary struct {
	Count  int           `json:"count"`
	P50    time.Duration `json:"p50"`
	P95    time.Duration `json:"p95"`
	P99    time.Duration `json:"p99"`
	Max    time.Duration `json:"max"`
	Errors int64         `json:"errors"`
}

type intervalReport struct {
	Kind        string             `json:"kind"`
	ElapsedSec  float64            `json:"elapsed_sec"`
	Requests    int                `json:"requests"`
	Successes   int64              `json:"successes"`
	Errors      int64              `json:"errors"`
	OpsPerSec   float64            `json:"ops_per_sec"`
	LatencyMS   map[string]float64 `json:"latency_ms"`
	Metrics     map[string]float64 `json:"midas_metrics,omitempty"`
	Postgres    map[string]float64 `json:"postgres,omitempty"`
	MetricsErr  string             `json:"metrics_error,omitempty"`
	PostgresErr string             `json:"postgres_error,omitempty"`
}

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseConfig(args []string) (config, error) {
	cfg := config{
		BaseURL:     "http://localhost:8080",
		DatabaseURL: getenvAny("DATABASE_URL", "MIDAS_DATABASE_URL"),
		Duration:    60 * time.Second,
		Concurrency: 1,
		Interval:    5 * time.Second,
		Timeout:     10 * time.Second,
		Seed:        true,
	}
	fs := flag.NewFlagSet("midas-loadtest", flag.ContinueOnError)
	fs.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL, "MIDAS HTTP base URL")
	fs.StringVar(&cfg.MetricsURL, "metrics-url", "", "Prometheus metrics URL; defaults to <base-url>/metrics")
	fs.StringVar(&cfg.DatabaseURL, "database-url", cfg.DatabaseURL, "Postgres DSN; required to prevent memory-mode benchmarking")
	fs.StringVar(&cfg.Token, "token", os.Getenv("MIDAS_LOAD_TOKEN"), "optional bearer token for /v1/evaluate")
	fs.DurationVar(&cfg.Duration, "duration", cfg.Duration, "load duration")
	fs.IntVar(&cfg.Concurrency, "concurrency", cfg.Concurrency, "concurrent workers")
	fs.IntVar(&cfg.Rate, "rate", 0, "optional global request rate per second; 0 means no client-side rate limit")
	fs.DurationVar(&cfg.Interval, "interval", cfg.Interval, "telemetry interval")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "per-request timeout")
	fs.BoolVar(&cfg.Seed, "seed", cfg.Seed, "seed minimal governed fixture into Postgres before load")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	return normalizeConfig(cfg)
}

func normalizeConfig(cfg config) (config, error) {
	if cfg.DatabaseURL == "" {
		return config{}, errors.New("database-url is required; refusing to benchmark without Postgres")
	}
	if cfg.Duration <= 0 {
		return config{}, errors.New("duration must be positive")
	}
	if cfg.Concurrency < 1 {
		return config{}, errors.New("concurrency must be at least 1")
	}
	if cfg.Rate < 0 {
		return config{}, errors.New("rate must be zero or positive")
	}
	if cfg.Interval <= 0 {
		return config{}, errors.New("interval must be positive")
	}
	if cfg.Timeout <= 0 {
		return config{}, errors.New("timeout must be positive")
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return config{}, fmt.Errorf("base-url must be absolute: %q", cfg.BaseURL)
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.MetricsURL == "" {
		cfg.MetricsURL = cfg.BaseURL + "/metrics"
	}
	metricsURL, err := url.Parse(cfg.MetricsURL)
	if err != nil || metricsURL.Scheme == "" || metricsURL.Host == "" {
		return config{}, fmt.Errorf("metrics-url must be absolute: %q", cfg.MetricsURL)
	}
	return cfg, nil
}

func run(ctx context.Context, cfg config, w io.Writer) error {
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer db.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	if cfg.Seed {
		if err := seedLoadData(ctx, db); err != nil {
			return err
		}
	}

	client := &http.Client{Timeout: cfg.Timeout}
	results := make(chan result, cfg.Concurrency*1024)
	var successes atomic.Int64
	var errorsCount atomic.Int64
	var seq atomic.Int64
	start := time.Now()
	stopAt := start.Add(cfg.Duration)

	var tokenC <-chan time.Time
	if cfg.Rate > 0 {
		ticker := time.NewTicker(time.Second / time.Duration(cfg.Rate))
		defer ticker.Stop()
		tokenC = ticker.C
	}

	var wg sync.WaitGroup
	wg.Add(cfg.Concurrency)
	for worker := 0; worker < cfg.Concurrency; worker++ {
		go func(workerID int) {
			defer wg.Done()
			for {
				if time.Now().After(stopAt) {
					return
				}
				if tokenC != nil {
					wait := time.Until(stopAt)
					if wait <= 0 {
						return
					}
					timer := time.NewTimer(wait)
					select {
					case <-timer.C:
						return
					case <-ctx.Done():
						timer.Stop()
						return
					case <-tokenC:
						timer.Stop()
					}
				}
				select {
				case <-ctx.Done():
					return
				default:
				}
				id := seq.Add(1)
				res := sendEvaluate(ctx, client, cfg, workerID, id)
				if res.Err == "" && res.Status >= 200 && res.Status < 300 {
					successes.Add(1)
				} else {
					errorsCount.Add(1)
				}
				select {
				case results <- res:
				case <-ctx.Done():
					return
				}
			}
		}(worker)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	latencies := make([]time.Duration, 0, 4096)
	lastN := 0
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case res, ok := <-results:
			if !ok {
				return emitReport(ctx, w, db, client, cfg, start, latencies, successes.Load(), errorsCount.Load(), "summary")
			}
			latencies = append(latencies, res.Duration)
		case <-ticker.C:
			currentN := len(latencies)
			window := append([]time.Duration(nil), latencies[lastN:currentN]...)
			lastN = currentN
			if err := emitInterval(ctx, w, db, client, cfg, start, window, successes.Load(), errorsCount.Load()); err != nil {
				return err
			}
		}
	}
}

func sendEvaluate(ctx context.Context, client *http.Client, cfg config, workerID int, seq int64) result {
	requestID := fmt.Sprintf("d38i-load-%d-%d-%d", time.Now().UnixNano(), workerID, seq)
	body := buildPayload(requestID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+"/v1/evaluate", bytes.NewReader(body))
	if err != nil {
		return result{Err: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return result{Duration: elapsed, Err: err.Error()}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return result{Duration: elapsed, Status: resp.StatusCode}
}

func buildPayload(requestID string) []byte {
	body := map[string]any{
		"surface_id":     loadSurfaceID,
		"process_id":     loadProcessID,
		"agent_id":       loadAgentID,
		"request_id":     requestID,
		"request_source": "d38i-loadtest",
		"confidence":     0.91,
		"context": map[string]any{
			"amount":     100,
			"channel":    "card",
			"risk_score": 0.12,
			"scenario":   "d38i-load-2",
		},
	}
	b, _ := json.Marshal(body)
	return b
}

func emitInterval(ctx context.Context, w io.Writer, db *sql.DB, client *http.Client, cfg config, start time.Time, latencies []time.Duration, successes, errs int64) error {
	return emitReport(ctx, w, db, client, cfg, start, latencies, successes, errs, "interval")
}

func emitReport(ctx context.Context, w io.Writer, db *sql.DB, client *http.Client, cfg config, start time.Time, latencies []time.Duration, successes, errs int64, kind string) error {
	elapsed := time.Since(start).Seconds()
	summary := summarize(latencies, errs)
	metrics, metricsErr := scrapeMetrics(ctx, client, cfg.MetricsURL)
	pg, pgErr := collectPostgresTelemetry(ctx, db)
	report := intervalReport{
		Kind:       kind,
		ElapsedSec: elapsed,
		Requests:   int(successes + errs),
		Successes:  successes,
		Errors:     errs,
		OpsPerSec:  float64(successes) / elapsed,
		LatencyMS: map[string]float64{
			"p50": toMillis(summary.P50),
			"p95": toMillis(summary.P95),
			"p99": toMillis(summary.P99),
			"max": toMillis(summary.Max),
		},
		Metrics:  metrics,
		Postgres: pg,
	}
	if metricsErr != nil {
		report.MetricsErr = metricsErr.Error()
	}
	if pgErr != nil {
		report.PostgresErr = pgErr.Error()
	}
	enc := json.NewEncoder(w)
	return enc.Encode(report)
}

func scrapeMetrics(ctx context.Context, client *http.Client, metricsURL string) (map[string]float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics status: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parsePrometheusSamples(string(body), []string{
		"midas_evaluation_duration_seconds",
		"midas_store_transaction_duration_seconds",
		"midas_store_transaction_stage_duration_seconds",
		"midas_database_pool_",
		"midas_outbox_",
		"midas_store_transaction_",
	}), nil
}

func parsePrometheusSamples(body string, prefixes []string) map[string]float64 {
	out := make(map[string]float64)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name := metricName(line)
		if name == "" || !hasPrefix(name, prefixes) {
			continue
		}
		if strings.HasSuffix(name, "_bucket") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		out[line[:strings.LastIndex(line, fields[1])-1]] = value
	}
	return out
}

func metricName(line string) string {
	if i := strings.IndexAny(line, "{ "); i >= 0 {
		return line[:i]
	}
	return line
}

func hasPrefix(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func collectPostgresTelemetry(ctx context.Context, db *sql.DB) (map[string]float64, error) {
	stats := make(map[string]float64)
	if err := db.QueryRowContext(ctx, `
SELECT
	COUNT(*) FILTER (WHERE state = 'active'),
	COUNT(*) FILTER (WHERE wait_event_type IS NOT NULL),
	COUNT(*)
FROM pg_stat_activity
WHERE datname = current_database()`).Scan(&scanFloat{stats, "pg_stat_activity_active"}, &scanFloat{stats, "pg_stat_activity_waiting"}, &scanFloat{stats, "pg_stat_activity_total"}); err != nil {
		return stats, err
	}
	_ = db.QueryRowContext(ctx, `
SELECT xact_commit, xact_rollback, deadlocks, temp_files
FROM pg_stat_database
WHERE datname = current_database()`).Scan(&scanFloat{stats, "pg_stat_database_xact_commit"}, &scanFloat{stats, "pg_stat_database_xact_rollback"}, &scanFloat{stats, "pg_stat_database_deadlocks"}, &scanFloat{stats, "pg_stat_database_temp_files"})
	_ = db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM pg_locks WHERE NOT granted`).Scan(&scanFloat{stats, "pg_locks_waiting"})
	_ = db.QueryRowContext(ctx, `
SELECT checkpoints_timed, checkpoints_req FROM pg_stat_bgwriter`).Scan(&scanFloat{stats, "pg_stat_bgwriter_checkpoints_timed"}, &scanFloat{stats, "pg_stat_bgwriter_checkpoints_req"})
	_ = db.QueryRowContext(ctx, `
SELECT wal_records, wal_fpi, wal_bytes FROM pg_stat_wal`).Scan(&scanFloat{stats, "pg_stat_wal_records"}, &scanFloat{stats, "pg_stat_wal_fpi"}, &scanFloat{stats, "pg_stat_wal_bytes"})
	return stats, nil
}

type scanFloat struct {
	m   map[string]float64
	key string
}

func (s *scanFloat) Scan(src any) error {
	switch v := src.(type) {
	case int64:
		s.m[s.key] = float64(v)
	case int32:
		s.m[s.key] = float64(v)
	case []byte:
		f, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return err
		}
		s.m[s.key] = f
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return err
		}
		s.m[s.key] = f
	default:
		return fmt.Errorf("unsupported scan type %T", src)
	}
	return nil
}

func summarize(durations []time.Duration, errs int64) latencySummary {
	if len(durations) == 0 {
		return latencySummary{Errors: errs}
	}
	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	pick := func(p float64) time.Duration {
		idx := int(p * float64(len(sorted)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return sorted[idx]
	}
	return latencySummary{
		Count:  len(sorted),
		P50:    pick(0.50),
		P95:    pick(0.95),
		P99:    pick(0.99),
		Max:    sorted[len(sorted)-1],
		Errors: errs,
	}
}

func toMillis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

func seedLoadData(ctx context.Context, db *sql.DB) error {
	if err := postgres.EnsureSchema(db); err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}
	pgStore, err := postgres.NewStore(db, nil)
	if err != nil {
		return fmt.Errorf("postgres store: %w", err)
	}
	repos, err := pgStore.Repositories()
	if err != nil {
		return fmt.Errorf("repositories: %w", err)
	}
	return seedRepositories(ctx, repos)
}

func seedRepositories(ctx context.Context, repos *store.Repositories) error {
	now := time.Now().UTC().Add(-time.Hour)
	if existing, err := repos.BusinessServices.GetByID(ctx, loadBusinessServiceID); err != nil {
		return fmt.Errorf("get business service: %w", err)
	} else if existing == nil {
		if err := repos.BusinessServices.Create(ctx, &businessservice.BusinessService{
			ID: loadBusinessServiceID, Name: "D38i load business service", ServiceType: businessservice.ServiceTypeInternal,
			Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("seed business service: %w", err)
		}
	}
	if existing, err := repos.Capabilities.GetByID(ctx, loadCapabilityID); err != nil {
		return fmt.Errorf("get capability: %w", err)
	} else if existing == nil {
		if err := repos.Capabilities.Create(ctx, &capability.Capability{
			ID: loadCapabilityID, Name: "D38i load capability", Status: "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("seed capability: %w", err)
		}
	}
	if existing, err := repos.Processes.GetByID(ctx, loadProcessID); err != nil {
		return fmt.Errorf("get process: %w", err)
	} else if existing == nil {
		if err := repos.Processes.Create(ctx, &process.Process{
			ID: loadProcessID, Name: "D38i load process", BusinessServiceID: loadBusinessServiceID,
			Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("seed process: %w", err)
		}
	}
	if existing, err := repos.Surfaces.FindLatestByID(ctx, loadSurfaceID); err != nil {
		return fmt.Errorf("find surface: %w", err)
	} else if existing == nil {
		if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
			ID: loadSurfaceID, Name: "D38i load surface", Status: surface.SurfaceStatusActive,
			Version: 1, EffectiveFrom: now, Domain: "test", BusinessOwner: "owner", TechnicalOwner: "tech",
			ProcessID: loadProcessID, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("seed surface: %w", err)
		}
	}
	if existing, err := repos.Agents.GetByID(ctx, loadAgentID); err != nil {
		return fmt.Errorf("get agent: %w", err)
	} else if existing == nil {
		if err := repos.Agents.Create(ctx, &agent.Agent{
			ID: loadAgentID, Name: "D38i load agent", Type: agent.AgentTypeAI,
			OperationalState: agent.OperationalStateActive, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("seed agent: %w", err)
		}
	}
	if existing, err := repos.Profiles.FindByID(ctx, loadProfileID); err != nil {
		return fmt.Errorf("find profile: %w", err)
	} else if existing == nil {
		if err := repos.Profiles.Create(ctx, &authority.AuthorityProfile{
			ID: loadProfileID, SurfaceID: loadSurfaceID, Name: "D38i load profile",
			Status: authority.ProfileStatusActive, ConfidenceThreshold: 0.5,
			ConsequenceThreshold: authority.Consequence{Type: value.ConsequenceTypeRiskRating, RiskRating: value.RiskRatingHigh},
			EscalationMode:       "auto", FailMode: authority.FailModeOpen, Version: 1, EffectiveDate: now,
		}); err != nil {
			return fmt.Errorf("seed profile: %w", err)
		}
	}
	if existing, err := repos.Grants.FindByID(ctx, loadGrantID); err != nil {
		return fmt.Errorf("find grant: %w", err)
	} else if existing == nil {
		if err := repos.Grants.Create(ctx, &authority.AuthorityGrant{
			ID: loadGrantID, AgentID: loadAgentID, ProfileID: loadProfileID,
			Status: authority.GrantStatusActive, EffectiveDate: now,
		}); err != nil {
			return fmt.Errorf("seed grant: %w", err)
		}
	}
	return nil
}

func getenvAny(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}
