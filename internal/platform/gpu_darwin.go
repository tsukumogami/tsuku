package platform

// DetectGPU returns "apple" unconditionally on macOS.
// Apple Silicon has an Apple GPU; Intel Macs have Metal-capable GPUs.
// No variant selection is needed on macOS.
func DetectGPU() string {
	return "apple"
}

// DetectGPUWithRoot is not used on macOS but exists for API consistency.
// Always returns "apple" regardless of root path.
func DetectGPUWithRoot(_ string) string {
	return "apple"
}
