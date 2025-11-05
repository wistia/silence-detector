package detector

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

const floatTolerance = 1e-6

func assertFloatEqual(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > floatTolerance {
		t.Fatalf("unexpected float value: got %f want %f", got, want)
	}
}

func TestDetectSilenceExecutesFFmpegAndParsesOutput(t *testing.T) {
	fakeOutput := `
[silencedetect @ 0x123] silence_start: 0.000000
[silencedetect @ 0x123] silence_end: 3.500000 | silence_duration: 3.500000
[silencedetect @ 0x123] silence_start: 10.000000
[silencedetect @ 0x123] silence_end: 12.000000 | silence_duration: 2.000000
frame=  100 fps=0.0 q=-0.0 size=       0kB time=00:00:12.00 bitrate=   0.0kbits/s speed=1x
`

	var capturedName string
	var capturedArgs []string

	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		capturedName = name
		capturedArgs = append([]string(nil), args...)
		return []byte(fakeOutput), nil
	}

	d := NewDetector(
		WithFFmpegPath("/usr/bin/ffmpeg-custom"),
		WithCommandRunner(runner),
	)

	result, err := d.DetectSilence(context.Background(), "video.mp4", DetectionOptions{
		NoiseLevel:         -25.5,
		MinSilenceDuration: 1.2,
	})
	if err != nil {
		t.Fatalf("DetectSilence returned error: %v", err)
	}

	if capturedName != "/usr/bin/ffmpeg-custom" {
		t.Fatalf("expected ffmpeg path %q, got %q", "/usr/bin/ffmpeg-custom", capturedName)
	}

	expectedFilter := "silencedetect=noise=-25.5dB:d=1.2"
	expectedArgs := []string{"-i", "video.mp4", "-af", expectedFilter, "-f", "null", "-"}
	if len(capturedArgs) != len(expectedArgs) {
		t.Fatalf("unexpected number of arguments: got %d, want %d (%v)", len(capturedArgs), len(expectedArgs), capturedArgs)
	}
	for i, arg := range expectedArgs {
		if capturedArgs[i] != arg {
			t.Fatalf("argument %d mismatch: got %q, want %q (all args: %v)", i, capturedArgs[i], arg, capturedArgs)
		}
	}

	if len(result.Intervals) != 2 {
		t.Fatalf("expected 2 intervals, got %d (%v)", len(result.Intervals), result.Intervals)
	}

	if math.Abs(result.InputDuration-12) > floatTolerance {
		t.Fatalf("unexpected input duration: got %f want %f", result.InputDuration, 12.0)
	}

	first := result.Intervals[0]
	assertFloatEqual(t, first.Start, 0)
	assertFloatEqual(t, first.End, 3.5)
	assertFloatEqual(t, first.Duration, 3.5)

	second := result.Intervals[1]
	assertFloatEqual(t, second.Start, 10)
	assertFloatEqual(t, second.End, 12)
	assertFloatEqual(t, second.Duration, 2)
}

func TestDetectSilenceValidatesDuration(t *testing.T) {
	d := NewDetector()
	_, err := d.DetectSilence(context.Background(), "video.mp4", DetectionOptions{
		NoiseLevel:         -30,
		MinSilenceDuration: 0,
	})
	if err == nil || !strings.Contains(err.Error(), "minimum silence duration") {
		t.Fatalf("expected minimum duration validation error, got %v", err)
	}
}

func TestParseSilenceIntervalsWithoutExplicitStart(t *testing.T) {
	output := `
[silencedetect @ 0x123] silence_end: 9.200000 | silence_duration: 2.000000
`

	intervals, _, err := parseSilenceOutput(output)
	if err != nil {
		t.Fatalf("parseSilenceIntervals returned error: %v", err)
	}

	if len(intervals) != 1 {
		t.Fatalf("expected 1 interval, got %d", len(intervals))
	}

	interval := intervals[0]
	assertFloatEqual(t, interval.Start, 7.2)
	assertFloatEqual(t, interval.End, 9.2)
	assertFloatEqual(t, interval.Duration, 2)
}

func TestDetectSilencePropagatesRunnerErrors(t *testing.T) {
	expectedErr := errors.New("boom")

	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("ffmpeg failure"), expectedErr
	}

	d := NewDetector(WithCommandRunner(runner))

	_, err := d.DetectSilence(context.Background(), "video.mp4", DetectionOptions{
		NoiseLevel:         -30,
		MinSilenceDuration: 1,
	})

	if err == nil || !strings.Contains(err.Error(), "ffmpeg execution failed") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestParseSilenceOutputUsesProgressForTrailingSilence(t *testing.T) {
	output := `
[silencedetect @ 0x123] silence_start: 0.000000
frame=   50 fps=0.0 q=-0.0 size=       0kB time=00:00:05.00 bitrate=   0.0kbits/s speed=1x
`

	intervals, duration, err := parseSilenceOutput(output)
	if err != nil {
		t.Fatalf("parseSilenceOutput returned error: %v", err)
	}

	if len(intervals) != 1 {
		t.Fatalf("expected 1 interval, got %d", len(intervals))
	}

	assertFloatEqual(t, duration, 5)

	interval := intervals[0]
	assertFloatEqual(t, interval.Start, 0)
	assertFloatEqual(t, interval.End, 5)
	assertFloatEqual(t, interval.Duration, 5)
}

func TestDetectionResultFullySilent(t *testing.T) {
	result := DetectionResult{
		InputDuration: 6,
		Intervals: []SilenceInterval{
			{Start: 0, End: 2, Duration: 2},
			{Start: 2, End: 4, Duration: 2},
			{Start: 4, End: 6, Duration: 2},
		},
	}

	if !result.FullySilent(1e-6) {
		t.Fatalf("expected fully silent input")
	}

	notSilent := DetectionResult{
		InputDuration: 5,
		Intervals: []SilenceInterval{
			{Start: 0, End: 1, Duration: 1},
		},
	}

	if notSilent.FullySilent(1e-6) {
		t.Fatalf("expected not fully silent input")
	}
}
