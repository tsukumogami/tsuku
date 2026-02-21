package platform

// ValidGPUTypes lists the recognized GPU vendor values.
// Detection returns one of these values based on GPU hardware present:
//   - nvidia: NVIDIA GPU (PCI vendor 0x10de)
//   - amd: AMD GPU (PCI vendor 0x1002)
//   - intel: Intel GPU (PCI vendor 0x8086)
//   - apple: Apple GPU (macOS, unconditional)
//   - none: no GPU detected or unsupported platform
var ValidGPUTypes = []string{"nvidia", "amd", "intel", "apple", "none"}
