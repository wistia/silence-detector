package main

import (
	"fmt"
	"os"

	"github.com/ssemakov/silence-detector/pkg/detector"
)

func main() {
	fmt.Println("Silence Detector")

	// Initialize the detector
	d := detector.NewDetector()

	if d == nil {
		fmt.Fprintln(os.Stderr, "Failed to initialize detector")
		os.Exit(1)
	}

	fmt.Println("Detector initialized successfully")
}
