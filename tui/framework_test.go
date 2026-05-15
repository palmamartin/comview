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

func TestConstraintsConstrainUnboundedMax(t *testing.T) {
	constraints := Constraints{
		Min: Size{Width: 10, Height: 5},
		Max: Size{Width: Unbounded, Height: Unbounded},
	}

	got := constraints.Constrain(Size{Width: 1000, Height: 2000})
	want := Size{Width: 1000, Height: 2000}
	if got != want {
		t.Fatalf("Constrain with unbounded max = %+v, want %+v", got, want)
	}
}

func TestConstraintsConstrainPartiallyUnboundedMax(t *testing.T) {
	constraints := Constraints{
		Max: Size{Width: 20, Height: Unbounded},
	}

	got := constraints.Constrain(Size{Width: 1000, Height: 2000})
	want := Size{Width: 20, Height: 2000}
	if got != want {
		t.Fatalf("Constrain with partially unbounded max = %+v, want %+v", got, want)
	}
}

func TestUnconstrained(t *testing.T) {
	constraints := Unconstrained()
	if !constraints.Max.HasUnboundedWidth() {
		t.Fatal("Unconstrained max width is bounded")
	}
	if !constraints.Max.HasUnboundedHeight() {
		t.Fatal("Unconstrained max height is bounded")
	}
}

func TestEditorCommandAddsLineArguments(t *testing.T) {
	tests := []struct {
		name     string
		editor   string
		target   EditorTarget
		wantName string
		wantArgs []string
		wantErr  bool
	}{
		{
			name:     "default vi",
			target:   EditorTarget{Path: "main.go", Line: 12, Column: 4},
			wantName: "vi",
			wantArgs: []string{"+call cursor(12,4)", "main.go"},
		},
		{
			name:     "editor with args",
			editor:   "nvim -p",
			target:   EditorTarget{Path: "main.go", Line: 12, Column: 4},
			wantName: "nvim",
			wantArgs: []string{"-p", "+call cursor(12,4)", "main.go"},
		},
		{
			name:     "quoted editor path",
			editor:   `"/opt/My Editor/bin/nvim" --clean`,
			target:   EditorTarget{Path: "main.go", Line: 12, Column: 4},
			wantName: "/opt/My Editor/bin/nvim",
			wantArgs: []string{"--clean", "+call cursor(12,4)", "main.go"},
		},
		{
			name:     "code",
			editor:   "code --reuse-window",
			target:   EditorTarget{Path: "main.go", Line: 12, Column: 4},
			wantName: "code",
			wantArgs: []string{"--reuse-window", "-g", "main.go:12:4"},
		},
		{
			name:     "unknown editor",
			editor:   "ed",
			target:   EditorTarget{Path: "main.go", Line: 12, Column: 4},
			wantName: "ed",
			wantArgs: []string{"main.go"},
		},
		{
			name:    "bad editor command",
			editor:  `"nvim`,
			target:  EditorTarget{Path: "main.go", Line: 12, Column: 4},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args, err := editorCommand(tt.editor, tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatal("err = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if name != tt.wantName {
				t.Fatalf("name = %q, want %q", name, tt.wantName)
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, tt.wantArgs)
			}
			for i := range args {
				if args[i] != tt.wantArgs[i] {
					t.Fatalf("args = %#v, want %#v", args, tt.wantArgs)
				}
			}
		})
	}
}
