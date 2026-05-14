package tui

import (
	"testing"
	"time"
)

func TestFramePipelineRendersFirstRequestImmediately(t *testing.T) {
	pipeline := NewFramePipeline(60)
	now := time.Unix(1, 0)
	frames := 0

	pipeline.request(now, func() {
		frames++
	})

	if frames != 1 {
		t.Fatalf("frames = %d, want 1", frames)
	}
	if pipeline.pending {
		t.Fatal("first request left a pending frame")
	}
}

func TestFramePipelineCoalescesUntilNextTick(t *testing.T) {
	pipeline := NewFramePipeline(60)
	now := time.Unix(1, 0)
	frames := 0

	render := func() {
		frames++
	}

	pipeline.request(now, render)
	pipeline.request(now.Add(time.Millisecond), render)
	pipeline.request(now.Add(2*time.Millisecond), render)

	if frames != 1 {
		t.Fatalf("frames before tick = %d, want 1", frames)
	}
	if !pipeline.pending {
		t.Fatal("coalesced requests did not leave a pending frame")
	}

	pipeline.tick(now.Add(pipeline.Interval()), render)

	if frames != 2 {
		t.Fatalf("frames after tick = %d, want 2", frames)
	}
	if pipeline.pending {
		t.Fatal("tick left a pending frame")
	}
}

func TestFramePipelineRendersImmediatelyAfterInterval(t *testing.T) {
	pipeline := NewFramePipeline(60)
	now := time.Unix(1, 0)
	frames := 0

	render := func() {
		frames++
	}

	pipeline.request(now, render)
	pipeline.request(now.Add(pipeline.Interval()), render)

	if frames != 2 {
		t.Fatalf("frames = %d, want 2", frames)
	}
	if pipeline.pending {
		t.Fatal("request after interval left a pending frame")
	}
}
