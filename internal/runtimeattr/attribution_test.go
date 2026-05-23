package runtimeattr

import (
	"testing"
	"time"
)

func TestNoOpRecorder(t *testing.T) {
	var r Recorder = NoOpRecorder{}
	r.RecordDuration(StageOrchestratorTotal, time.Millisecond)
	r.AddCount(CountAuditAppend, 1)
	ObserveValue(r, ValueAuditPayloadBytes, 42)
}

func TestCollectorAggregatesDurationsCountsAndValues(t *testing.T) {
	c := NewCollector()
	c.RecordDuration(StageAuditAppend, 2*time.Millisecond)
	c.RecordDuration(StageAuditAppend, 4*time.Millisecond)
	c.AddCount(CountAuditAppend, 2)
	c.AddCount(CountOutboxAppend, 3)
	c.AddValue(ValueAuditPayloadBytes, 100)
	c.AddValue(ValueAuditPayloadBytes, 140)
	ObserveValue(c, AuditPayloadBytesByTypeValue("decision.accepted"), 90)

	snap := c.Snapshot()
	stats := snap.Durations[StageAuditAppend]
	if stats.Count != 2 {
		t.Fatalf("duration count: got %d, want 2", stats.Count)
	}
	if stats.Total != 6*time.Millisecond {
		t.Fatalf("duration total: got %s, want 6ms", stats.Total)
	}
	if stats.Average() != 3*time.Millisecond {
		t.Fatalf("duration average: got %s, want 3ms", stats.Average())
	}
	if snap.Counts[CountAuditAppend] != 2 {
		t.Fatalf("audit count: got %d, want 2", snap.Counts[CountAuditAppend])
	}
	if snap.Counts[CountOutboxAppend] != 3 {
		t.Fatalf("outbox count: got %d, want 3", snap.Counts[CountOutboxAppend])
	}
	valueStats := snap.Values[ValueAuditPayloadBytes]
	if valueStats.Count != 2 {
		t.Fatalf("value count: got %d, want 2", valueStats.Count)
	}
	if valueStats.Total != 240 {
		t.Fatalf("value total: got %d, want 240", valueStats.Total)
	}
	if valueStats.Average() != 120 {
		t.Fatalf("value average: got %d, want 120", valueStats.Average())
	}
	if valueStats.Max != 140 {
		t.Fatalf("value max: got %d, want 140", valueStats.Max)
	}
	if got := snap.Values[AuditPayloadBytesByTypeValue("decision.accepted")].Average(); got != 90 {
		t.Fatalf("typed payload bytes average: got %d, want 90", got)
	}
}

func TestCollectorReset(t *testing.T) {
	c := NewCollector()
	c.RecordDuration(StagePolicyEvaluation, time.Millisecond)
	c.AddCount(CountAuditAppend, 1)
	c.Reset()

	snap := c.Snapshot()
	if len(snap.Durations) != 0 {
		t.Fatalf("durations after reset: got %d, want 0", len(snap.Durations))
	}
	if len(snap.Counts) != 0 {
		t.Fatalf("counts after reset: got %d, want 0", len(snap.Counts))
	}
	if len(snap.Values) != 0 {
		t.Fatalf("values after reset: got %d, want 0", len(snap.Values))
	}
}
