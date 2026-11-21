package main

import (
	"slices"
	"testing"
	"time"
)

func TestGenerationBatchDispatchHonorsMode(t *testing.T) {
	runs := []NativeRun{{ID: "first"}, {ID: "second"}, {ID: "third"}}
	var serial []string
	slots := make(chan struct{}, 4)
	dispatchGenerationBatch(runs, false, slots, func(r NativeRun) { serial = append(serial, r.ID) })
	if !slices.Equal(serial, []string{"first", "second", "third"}) {
		t.Fatal("serial order", serial)
	}

	started := make(chan string, len(runs))
	finish := make(chan struct{})
	defer close(finish)
	go dispatchGenerationBatch(runs, true, slots, func(r NativeRun) {
		started <- r.ID
		<-finish
	})
	var parallel []string
	deadline := time.After(2 * time.Second)
	for range runs {
		select {
		case id := <-started:
			parallel = append(parallel, id)
		case <-deadline:
			t.Fatal("parallel runs did not start together", parallel)
		}
	}
	slices.Sort(parallel)
	if !slices.Equal(parallel, serial) {
		t.Fatal("parallel run set", parallel)
	}
}
