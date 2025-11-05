package detector

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// CommandRunner defines a function capable of executing an external command and returning its combined output.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// SilenceInterval captures the start, end, and duration of a detected silent period.
type SilenceInterval struct {
	Start    float64
	End      float64
	Duration float64
}

// DetectionOptions configures how ffmpeg performs silence detection.
type DetectionOptions struct {
	NoiseLevel         float64
	MinSilenceDuration float64
}

// DetectionResult captures the detected silence intervals alongside metadata about the input file.
type DetectionResult struct {
	Intervals     []SilenceInterval
	InputDuration float64
}

// FullySilent reports whether the detected silence intervals span the entire input duration.
//
// The tolerance parameter allows a small slack when comparing floating point timestamps and durations.
func (r DetectionResult) FullySilent(tolerance float64) bool {
	if r.InputDuration <= 0 || len(r.Intervals) == 0 {
		return false
	}

	first := r.Intervals[0]
	if first.Start > tolerance {
		return false
	}

	prevEnd := first.End
	for _, interval := range r.Intervals[1:] {
		if interval.Start-prevEnd > tolerance {
			return false
		}
		prevEnd = interval.End
	}

	last := r.Intervals[len(r.Intervals)-1]
	if math.Abs(last.End-r.InputDuration) > tolerance {
		return false
	}

	return true
}

// Detector orchestrates executing ffmpeg and parsing its silence detection output.
type Detector struct {
	ffmpegPath string
	run        CommandRunner
}

// Option customises the Detector during construction.
type Option func(*Detector)

// WithFFmpegPath overrides the ffmpeg binary path used by the detector.
func WithFFmpegPath(path string) Option {
	return func(d *Detector) {
		d.ffmpegPath = path
	}
}

// WithCommandRunner overrides the command execution function used by the detector.
func WithCommandRunner(runner CommandRunner) Option {
	return func(d *Detector) {
		d.run = runner
	}
}

// NewDetector creates a detector with default configuration.
func NewDetector(opts ...Option) *Detector {
	d := &Detector{
		ffmpegPath: "ffmpeg",
		run:        defaultCommandRunner,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// DetectSilence executes ffmpeg with the silencedetect audio filter and parses the resulting intervals.
func (d *Detector) DetectSilence(ctx context.Context, inputPath string, options DetectionOptions) (DetectionResult, error) {
	if inputPath == "" {
		return DetectionResult{}, errors.New("input path is required")
	}

	if options.MinSilenceDuration <= 0 {
		return DetectionResult{}, fmt.Errorf("minimum silence duration must be greater than zero, got %f", options.MinSilenceDuration)
	}

	noiseLevel := strconv.FormatFloat(options.NoiseLevel, 'f', -1, 64)
	minDuration := strconv.FormatFloat(options.MinSilenceDuration, 'f', -1, 64)

	filter := fmt.Sprintf("silencedetect=noise=%sdB:d=%s", noiseLevel, minDuration)

	args := []string{"-i", inputPath, "-af", filter, "-f", "null", "-"}

	output, err := d.run(ctx, d.ffmpegPath, args...)
	if err != nil {
		return DetectionResult{}, fmt.Errorf("ffmpeg execution failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	intervals, duration, err := parseSilenceOutput(string(output))
	if err != nil {
		return DetectionResult{}, err
	}

	return DetectionResult{Intervals: intervals, InputDuration: duration}, nil
}

var (
	silenceStartPattern = regexp.MustCompile(`silence_start:\s*([0-9]+(?:\.[0-9]+)?)`)
	silenceEndPattern   = regexp.MustCompile(`silence_end:\s*([0-9]+(?:\.[0-9]+)?)\s*\|\s*silence_duration:\s*([0-9]+(?:\.[0-9]+)?)`)
	progressTimePattern = regexp.MustCompile(`time=([0-9]{2}):([0-9]{2}):([0-9]+(?:\.[0-9]+)?)`)
)

func parseSilenceOutput(output string) ([]SilenceInterval, float64, error) {
	var intervals []SilenceInterval
	var currentStart *float64
	var lastProgress float64

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if matches := silenceStartPattern.FindStringSubmatch(line); len(matches) == 2 {
			start, err := strconv.ParseFloat(matches[1], 64)
			if err != nil {
				return nil, 0, fmt.Errorf("parse silence start: %w", err)
			}
			currentStart = &start
			continue
		}

		if matches := silenceEndPattern.FindStringSubmatch(line); len(matches) == 3 {
			end, err := strconv.ParseFloat(matches[1], 64)
			if err != nil {
				return nil, 0, fmt.Errorf("parse silence end: %w", err)
			}
			duration, err := strconv.ParseFloat(matches[2], 64)
			if err != nil {
				return nil, 0, fmt.Errorf("parse silence duration: %w", err)
			}

			start := end - duration
			if currentStart != nil {
				start = *currentStart
			}

			intervals = append(intervals, SilenceInterval{
				Start:    start,
				End:      end,
				Duration: duration,
			})

			currentStart = nil
			continue
		}

		if matches := progressTimePattern.FindStringSubmatch(line); len(matches) == 4 {
			hours, err := strconv.Atoi(matches[1])
			if err != nil {
				return nil, 0, fmt.Errorf("parse progress hours: %w", err)
			}
			minutes, err := strconv.Atoi(matches[2])
			if err != nil {
				return nil, 0, fmt.Errorf("parse progress minutes: %w", err)
			}
			seconds, err := strconv.ParseFloat(matches[3], 64)
			if err != nil {
				return nil, 0, fmt.Errorf("parse progress seconds: %w", err)
			}

			lastProgress = float64(hours*3600+minutes*60) + seconds
		}
	}

	if currentStart != nil && lastProgress > *currentStart {
		start := *currentStart
		end := lastProgress
		intervals = append(intervals, SilenceInterval{
			Start:    start,
			End:      end,
			Duration: end - start,
		})
	}

	return intervals, lastProgress, nil
}

func defaultCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}
