package platform

// DetectGPU returns "none" on Windows.
// Only CPU variants are built for Windows currently.
func DetectGPU() string {
	return "none"
}

// DetectGPUWithRoot is not used on Windows but exists for API consistency.
// Always returns "none" regardless of root path.
func DetectGPUWithRoot(_ string) string {
	return "none"
}
