package detector

// Detector represents a silence detector instance
type Detector struct {
	threshold float64
}

// NewDetector creates a new Detector instance with default settings
func NewDetector() *Detector {
	return &Detector{
		threshold: 0.01, // Default threshold for silence detection
	}
}

// SetThreshold sets the silence detection threshold
func (d *Detector) SetThreshold(threshold float64) {
	d.threshold = threshold
}

// GetThreshold returns the current silence detection threshold
func (d *Detector) GetThreshold() float64 {
	return d.threshold
}

// IsSilent checks if the given audio level is below the silence threshold
func (d *Detector) IsSilent(level float64) bool {
	return level < d.threshold
}
