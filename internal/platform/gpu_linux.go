package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// PCI class codes for display controllers (top 16 bits).
const (
	pciClassVGA = "0x0300" // VGA compatible controller
	pciClass3D  = "0x0302" // 3D controller (e.g., NVIDIA Tesla)
)

// PCI vendor IDs for GPU manufacturers.
var pciVendorToGPU = map[string]string{
	"0x10de": "nvidia",
	"0x1002": "amd",
	"0x8086": "intel",
}

// gpuPriority defines preference order when multiple GPUs are present.
// Lower index = higher priority. NVIDIA discrete > AMD discrete > Intel integrated.
var gpuPriority = map[string]int{
	"nvidia": 0,
	"amd":    1,
	"intel":  2,
}

// DetectGPU returns the primary GPU vendor for the current system.
// Returns one of: "nvidia", "amd", "intel", "none".
//
// On Linux, scans PCI devices via sysfs for display controllers.
// When multiple GPUs are present, prefers discrete over integrated
// (nvidia > amd > intel).
func DetectGPU() string {
	return DetectGPUWithRoot("")
}

// DetectGPUWithRoot detects the GPU vendor using a custom root path for testing.
// An empty root uses the real filesystem root.
func DetectGPUWithRoot(root string) string {
	if root == "" {
		root = "/"
	}
	pattern := filepath.Join(root, "sys", "bus", "pci", "devices", "*", "class")
	classFiles, err := filepath.Glob(pattern)
	if err != nil || len(classFiles) == 0 {
		return "none"
	}

	bestGPU := ""
	bestPriority := len(gpuPriority) // worse than any known GPU

	for _, classFile := range classFiles {
		classData, err := os.ReadFile(classFile)
		if err != nil {
			continue
		}

		classStr := strings.TrimSpace(string(classData))
		if !isDisplayController(classStr) {
			continue
		}

		// Read vendor file from the same device directory
		deviceDir := filepath.Dir(classFile)
		vendorFile := filepath.Join(deviceDir, "vendor")
		vendorData, err := os.ReadFile(vendorFile)
		if err != nil {
			continue
		}

		vendorStr := strings.TrimSpace(string(vendorData))
		gpu, ok := pciVendorToGPU[vendorStr]
		if !ok {
			continue
		}

		priority, ok := gpuPriority[gpu]
		if !ok {
			continue
		}

		if priority < bestPriority {
			bestGPU = gpu
			bestPriority = priority
		}
	}

	if bestGPU == "" {
		return "none"
	}
	return bestGPU
}

// isDisplayController checks if a PCI class code represents a display controller.
// Class codes are in the format "0xCCSSPP" where CC=class, SS=subclass, PP=prog-if.
// We check the top 16 bits (class+subclass): 0x0300 for VGA, 0x0302 for 3D.
func isDisplayController(classStr string) bool {
	if len(classStr) < 6 {
		return false
	}
	prefix := classStr[:6] // "0x0300" or "0x0302"
	return prefix == pciClassVGA || prefix == pciClass3D
}
