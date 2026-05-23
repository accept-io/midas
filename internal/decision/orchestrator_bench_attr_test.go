package decision_test

import (
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/runtimeattr"
)

func reportInlineBenchAttribution(b *testing.B, c *runtimeattr.Collector, iterations float64) {
	b.Helper()
	if iterations <= 0 {
		return
	}
	snap := c.Snapshot()
	for _, stage := range snap.Stages() {
		stats := snap.Durations[stage]
		if stats.Count == 0 {
			continue
		}
		unit := "attr_" + sanitizeInlineBenchMetricName(string(stage)) + "_avg_us"
		b.ReportMetric(float64(stats.Average().Microseconds()), unit)
	}
	for _, name := range snap.CountNames() {
		unit := "attr_" + sanitizeInlineBenchMetricName(string(name)) + "_per_op"
		b.ReportMetric(float64(snap.Counts[name])/iterations, unit)
	}
	for _, name := range snap.ValueNames() {
		stats := snap.Values[name]
		if stats.Count == 0 {
			continue
		}
		base := "attr_" + sanitizeInlineBenchMetricName(string(name))
		b.ReportMetric(float64(stats.Average()), base+"_avg_bytes")
		b.ReportMetric(float64(stats.Max), base+"_max_bytes")
		b.ReportMetric(float64(stats.Count)/iterations, base+"_count_per_op")
	}
}

func sanitizeInlineBenchMetricName(s string) string {
	replacer := strings.NewReplacer(".", "_", "-", "_", "/", "_", " ", "_")
	return replacer.Replace(s)
}
