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

func TestConstraintsConstrain(t *testing.T) {
	constraints := Constraints{
		Min: Size{Width: 10, Height: 5},
		Max: Size{Width: 20, Height: 15},
	}

	tests := []struct {
		name string
		size Size
		want Size
	}{
		{
			name: "below min",
			size: Size{Width: 1, Height: 2},
			want: Size{Width: 10, Height: 5},
		},
		{
			name: "above max",
			size: Size{Width: 30, Height: 40},
			want: Size{Width: 20, Height: 15},
		},
		{
			name: "inside",
			size: Size{Width: 12, Height: 8},
			want: Size{Width: 12, Height: 8},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := constraints.Constrain(test.size); got != test.want {
				t.Fatalf("Constrain(%+v) = %+v, want %+v", test.size, got, test.want)
			}
		})
	}
}
