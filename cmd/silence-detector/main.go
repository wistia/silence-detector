package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ssemakov/silence-detector/pkg/detector"
)

type outputFormat string

const (
	outputFormatText outputFormat = "text"
	outputFormatJSON outputFormat = "json"
)

func main() {
	var (
		inputPath        = flag.String("input", "", "Path to the input media file (required)")
		noiseLevel       = flag.Float64("silence-noise", -30, "Silence noise threshold in dB")
		minDuration      = flag.Float64("silence-duration", 0.5, "Minimum silence duration in seconds")
		format           = flag.String("output", string(outputFormatText), "Output format: text or json")
		ffmpegBinary     = flag.String("ffmpeg", "ffmpeg", "Path to the ffmpeg binary")
		checkFullSilence = flag.Bool("check-full-silence", false, "Report whether the entire input is silent")
	)

	flag.Parse()

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "--input flag is required")
		flag.Usage()
		os.Exit(1)
	}

	originalInput := strings.TrimSpace(*inputPath)
	resolvedInput := originalInput
	var cleanup func()

	if isRemoteInput(resolvedInput) {
		downloadedPath, c, err := downloadRemoteInput(resolvedInput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to download input %q: %v\n", originalInput, err)
			os.Exit(1)
		}
		resolvedInput = downloadedPath
		cleanup = c
	}

	if cleanup != nil {
		defer cleanup()
	}

	if info, err := os.Stat(resolvedInput); err != nil {
		if cleanup != nil {
			cleanup()
		}
		fmt.Fprintf(os.Stderr, "failed to stat input %q: %v\n", originalInput, err)
		os.Exit(1)
	} else if info.IsDir() {
		if cleanup != nil {
			cleanup()
		}
		fmt.Fprintf(os.Stderr, "input %q is a directory, expected a file\n", resolvedInput)
		os.Exit(1)
	}

	if *minDuration <= 0 {
		fmt.Fprintln(os.Stderr, "--silence-duration must be greater than zero")
		os.Exit(1)
	}

	requestedFormat := outputFormat(strings.ToLower(strings.TrimSpace(*format)))
	if requestedFormat != outputFormatText && requestedFormat != outputFormatJSON {
		fmt.Fprintf(os.Stderr, "unsupported output format %q\n", *format)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	det := detector.NewDetector(detector.WithFFmpegPath(*ffmpegBinary))

	result, err := det.DetectSilence(ctx, resolvedInput, detector.DetectionOptions{
		NoiseLevel:         *noiseLevel,
		MinSilenceDuration: *minDuration,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "silence detection failed: %v\n", err)
		os.Exit(1)
	}

	if *checkFullSilence && result.InputDuration <= 0 {
		fmt.Fprintln(os.Stderr, "ffmpeg output did not include duration information; cannot determine full silence")
		os.Exit(1)
	}

	switch requestedFormat {
	case outputFormatJSON:
		emitJSON(result, *inputPath, *noiseLevel, *minDuration, *checkFullSilence)
	default:
		emitText(result, *inputPath, *noiseLevel, *minDuration, *checkFullSilence)
	}
}

func emitJSON(result detector.DetectionResult, inputPath string, noiseLevel, minDuration float64, checkFullSilence bool) {
	report := struct {
		Input       string                     `json:"input"`
		NoiseDB     float64                    `json:"noise_db"`
		MinDur      float64                    `json:"min_duration"`
		Duration    float64                    `json:"duration"`
		FullySilent *bool                      `json:"fully_silent,omitempty"`
		Intervals   []detector.SilenceInterval `json:"intervals"`
	}{
		Input:     displayInputPath(inputPath),
		NoiseDB:   noiseLevel,
		MinDur:    minDuration,
		Duration:  result.InputDuration,
		Intervals: result.Intervals,
	}

	if checkFullSilence {
		fullySilent := result.FullySilent(1e-3)
		report.FullySilent = &fullySilent
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
}

func emitText(result detector.DetectionResult, inputPath string, noiseLevel, minDuration float64, checkFullSilence bool) {
	fmt.Printf("Silence detection for %s\n", displayInputPath(inputPath))
	fmt.Printf("Noise threshold: %.2fdB, Minimum duration: %.2fs\n", noiseLevel, minDuration)
	if result.InputDuration > 0 {
		fmt.Printf("Input duration: %.3fs\n", result.InputDuration)
	}

	if len(result.Intervals) == 0 {
		fmt.Println("No silence intervals detected.")
		if checkFullSilence {
			fmt.Println("Entire file is not silent.")
		}
		return
	}

	fmt.Printf("Detected %d silence interval(s):\n", len(result.Intervals))
	for i, interval := range result.Intervals {
		fmt.Printf("%d. start=%.3fs end=%.3fs duration=%.3fs\n", i+1, interval.Start, interval.End, interval.Duration)
	}

	if checkFullSilence {
		if result.FullySilent(1e-3) {
			fmt.Println("Entire file is silent.")
		} else {
			fmt.Println("Entire file is not silent.")
		}
	}
}

func isRemoteInput(path string) bool {
	if path == "" {
		return false
	}

	parsed, err := url.Parse(path)
	if err != nil {
		return false
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func displayInputPath(path string) string {
	if isRemoteInput(path) {
		return path
	}
	return filepath.Clean(path)
}

func downloadRemoteInput(rawURL string) (string, func(), error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", nil, fmt.Errorf("invalid URL: %w", err)
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(rawURL)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", nil, fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}

	ext := filepath.Ext(parsed.Path)
	tmpFile, err := os.CreateTemp("", "silence-detector-*"+ext)
	if err != nil {
		return "", nil, err
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", nil, err
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", nil, err
	}

	cleanup := func() {
		os.Remove(tmpFile.Name())
	}

	return tmpFile.Name(), cleanup, nil
}
