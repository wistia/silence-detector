package detector

import "testing"

func TestNewDetector(t *testing.T) {
	d := NewDetector()
	if d == nil {
		t.Fatal("NewDetector() returned nil")
	}

	expectedThreshold := 0.01
	if d.GetThreshold() != expectedThreshold {
		t.Errorf("Expected threshold %f, got %f", expectedThreshold, d.GetThreshold())
	}
}

func TestSetThreshold(t *testing.T) {
	d := NewDetector()

	testThreshold := 0.05
	d.SetThreshold(testThreshold)

	if d.GetThreshold() != testThreshold {
		t.Errorf("Expected threshold %f, got %f", testThreshold, d.GetThreshold())
	}
}

func TestIsSilent(t *testing.T) {
	d := NewDetector()
	d.SetThreshold(0.01)

	tests := []struct {
		name     string
		level    float64
		expected bool
	}{
		{"Below threshold", 0.005, true},
		{"At threshold", 0.01, false},
		{"Above threshold", 0.02, false},
		{"Zero level", 0.0, true},
		{"High level", 0.5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.IsSilent(tt.level)
			if result != tt.expected {
				t.Errorf("IsSilent(%f) = %v, expected %v", tt.level, result, tt.expected)
			}
		})
	}
}
