package dispatch

import "time"

// Recorder observes dispatcher internals without coupling the dispatcher to a
// concrete metrics backend.
type Recorder interface {
	RecordClaimDuration(time.Duration)
	RecordPublishDuration(topic string, d time.Duration)
	RecordMarkPublishedDuration(time.Duration)
	AddClaimed(n int)
	AddPublished(n int)
	IncrementPublishFailure(topic string, errorClass string)
	IncrementMarkPublishedFailure()
	ObserveBatchSize(n int)
}

type noopRecorder struct{}

func (noopRecorder) RecordClaimDuration(time.Duration)           {}
func (noopRecorder) RecordPublishDuration(string, time.Duration) {}
func (noopRecorder) RecordMarkPublishedDuration(time.Duration)   {}
func (noopRecorder) AddClaimed(int)                              {}
func (noopRecorder) AddPublished(int)                            {}
func (noopRecorder) IncrementPublishFailure(string, string)      {}
func (noopRecorder) IncrementMarkPublishedFailure()              {}
func (noopRecorder) ObserveBatchSize(int)                        {}

var _ Recorder = noopRecorder{}
