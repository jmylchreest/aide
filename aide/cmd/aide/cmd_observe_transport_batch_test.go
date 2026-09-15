package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

type countingObserveBatch struct {
	store.ObserveEventStore
	batches []int
	fail    bool
}

func (s *countingObserveBatch) AddObserveEvent(*observe.Event) error {
	panic("unexpected single write")
}
func (s *countingObserveBatch) AddObserveEvents(events []*observe.Event) ([]bool, error) {
	s.batches = append(s.batches, len(events))
	if s.fail {
		return nil, fmt.Errorf("write failed")
	}
	return make([]bool, len(events)), nil // All accepted duplicates still acknowledge.
}
func TestObserveStreamBatchesAndAcknowledgesDuplicates(t *testing.T) {
	sink := &countingObserveBatch{}
	input := strings.Repeat("{\"kind\":\"session\",\"name\":\"model_usage\"}\n", 600) + "bad\n"
	recorded, skipped, err := recordObserveStream(strings.NewReader(input), sink)
	if err != nil || recorded != 600 || skipped != 1 || fmt.Sprint(sink.batches) != "[256 256 88]" {
		t.Fatalf("recorded=%d skipped=%d batches=%v err=%v", recorded, skipped, sink.batches, err)
	}
}
func TestObserveStreamBoundsBytesAndRejectsUnacknowledgedBatch(t *testing.T) {
	line := "{\"kind\":\"session\",\"name\":\"test\",\"attrs\":{\"large\":\"" + strings.Repeat("x", 600000) + "\"}}\n"
	sink := &countingObserveBatch{fail: true}
	recorded, skipped, err := recordObserveStream(strings.NewReader(line+line), sink)
	if err != nil || recorded != 0 || skipped != 2 || fmt.Sprint(sink.batches) != "[1 1]" {
		t.Fatalf("recorded=%d skipped=%d batches=%v err=%v", recorded, skipped, sink.batches, err)
	}
}
