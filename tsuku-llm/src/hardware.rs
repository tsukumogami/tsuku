//! Hardware detection for the local LLM runtime.
//!
//! Detects GPU capabilities, memory resources, and CPU features to inform model selection.
//! Runs once at server startup.

use std::path::Path;
use tracing::{debug, info, warn};

/// Available GPU backends in priority order.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum GpuBackend {
    /// NVIDIA CUDA (highest priority on supported systems)
    Cuda,
    /// Apple Metal (macOS ARM)
    Metal,
    /// Vulkan (AMD, Intel, or NVIDIA fallback)
    Vulkan,
    /// No GPU acceleration available
    None,
}

impl std::fmt::Display for GpuBackend {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            GpuBackend::Cuda => write!(f, "cuda"),
            GpuBackend::Metal => write!(f, "metal"),
            GpuBackend::Vulkan => write!(f, "vulkan"),
            GpuBackend::None => write!(f, "cpu"),
        }
    }
}

/// CPU instruction set features relevant to inference performance.
#[derive(Debug, Clone, Default)]
pub struct CpuFeatures {
    /// AVX2 support (baseline for modern x86_64)
    pub avx2: bool,
    /// AVX-512 support (faster matrix ops on supported Intel/AMD)
    pub avx512: bool,
}

/// Complete hardware profile for model selection.
#[derive(Debug, Clone)]
pub struct HardwareProfile {
    /// Best available GPU backend
    pub gpu_backend: GpuBackend,
    /// Available video memory in bytes (0 for CPU-only)
    pub vram_bytes: u64,
    /// System RAM in bytes
    pub ram_bytes: u64,
    /// CPU instruction set features
    pub cpu_features: CpuFeatures,
}

impl Default for HardwareProfile {
    fn default() -> Self {
        Self {
            gpu_backend: GpuBackend::None,
            vram_bytes: 0,
            ram_bytes: 0,
            cpu_features: CpuFeatures::default(),
        }
    }
}

/// Detects hardware capabilities for model selection.
pub struct HardwareDetector;

impl HardwareDetector {
    /// Detect all hardware capabilities and return a complete profile.
    pub fn detect() -> HardwareProfile {
        info!("Starting hardware detection");

        let cpu_features = Self::detect_cpu_features();
        debug!("CPU features: avx2={}, avx512={}", cpu_features.avx2, cpu_features.avx512);

        let ram_bytes = Self::detect_system_ram();
        debug!("System RAM: {} bytes ({:.1} GB)", ram_bytes, ram_bytes as f64 / 1e9);

        let (gpu_backend, vram_bytes) = Self::detect_gpu();
        debug!("GPU backend: {:?}, VRAM: {} bytes ({:.1} GB)",
               gpu_backend, vram_bytes, vram_bytes as f64 / 1e9);

        let profile = HardwareProfile {
            gpu_backend,
            vram_bytes,
            ram_bytes,
            cpu_features,
        };

        info!(
            "Hardware profile: backend={}, vram={:.1}GB, ram={:.1}GB, avx2={}, avx512={}",
            profile.gpu_backend,
            profile.vram_bytes as f64 / 1e9,
            profile.ram_bytes as f64 / 1e9,
            profile.cpu_features.avx2,
            profile.cpu_features.avx512
        );

        profile
    }

    /// Detect GPU backend and VRAM.
    /// Priority: CUDA > Metal > Vulkan > None
    fn detect_gpu() -> (GpuBackend, u64) {
        // Try CUDA first (NVIDIA)
        if let Some(vram) = Self::detect_cuda() {
            return (GpuBackend::Cuda, vram);
        }

        // Try Metal (macOS ARM)
        if let Some(vram) = Self::detect_metal() {
            return (GpuBackend::Metal, vram);
        }

        // Try Vulkan (AMD, Intel, NVIDIA fallback)
        if let Some(vram) = Self::detect_vulkan() {
            return (GpuBackend::Vulkan, vram);
        }

        // No GPU available
        (GpuBackend::None, 0)
    }

    /// Detect NVIDIA CUDA availability by probing for the CUDA library.
    fn detect_cuda() -> Option<u64> {
        #[cfg(target_os = "linux")]
        {
            // Check for CUDA driver library
            let cuda_paths = [
                "/usr/lib/x86_64-linux-gnu/libcuda.so",
                "/usr/lib/x86_64-linux-gnu/libcuda.so.1",
                "/usr/lib64/libcuda.so",
                "/usr/lib64/libcuda.so.1",
                "/usr/local/cuda/lib64/libcuda.so",
            ];

            for path in &cuda_paths {
                if Path::new(path).exists() {
                    debug!("Found CUDA library at {}", path);
                    // Try to get VRAM info via nvidia-smi
                    let vram = Self::get_nvidia_vram().unwrap_or(0);
                    return Some(vram);
                }
            }
        }

        #[cfg(target_os = "windows")]
        {
            // Check for CUDA driver DLL
            let cuda_paths = [
                "C:\\Windows\\System32\\nvcuda.dll",
            ];

            for path in &cuda_paths {
                if Path::new(path).exists() {
                    debug!("Found CUDA library at {}", path);
                    let vram = Self::get_nvidia_vram().unwrap_or(0);
                    return Some(vram);
                }
            }
        }

        None
    }

    /// Get NVIDIA VRAM via nvidia-smi command.
    fn get_nvidia_vram() -> Option<u64> {
        let output = std::process::Command::new("nvidia-smi")
            .args(["--query-gpu=memory.total", "--format=csv,noheader,nounits"])
            .output()
            .ok()?;

        if !output.status.success() {
            return None;
        }

        let stdout = String::from_utf8_lossy(&output.stdout);
        // nvidia-smi reports in MiB, take the first GPU's memory
        let mib: u64 = stdout.lines().next()?.trim().parse().ok()?;
        Some(mib * 1024 * 1024)
    }

    /// Detect Apple Metal availability (macOS ARM only).
    fn detect_metal() -> Option<u64> {
        #[cfg(all(target_os = "macos", target_arch = "aarch64"))]
        {
            // On Apple Silicon, Metal is always available and uses unified memory
            debug!("Apple Silicon detected, Metal available");
            // Return system RAM as VRAM (unified memory architecture)
            let ram = Self::detect_system_ram();
            // Report ~75% of RAM as available for GPU use (conservative estimate)
            return Some(ram * 3 / 4);
        }

        #[cfg(all(target_os = "macos", target_arch = "x86_64"))]
        {
            // Intel Macs may have discrete GPUs with Metal support
            // but we primarily target Apple Silicon for Metal
            debug!("Intel Mac detected, skipping Metal (prefer Vulkan for discrete GPU)");
        }

        None
    }

    /// Detect Vulkan availability by probing for the Vulkan library.
    fn detect_vulkan() -> Option<u64> {
        #[cfg(target_os = "linux")]
        {
            let vulkan_paths = [
                "/usr/lib/x86_64-linux-gnu/libvulkan.so",
                "/usr/lib/x86_64-linux-gnu/libvulkan.so.1",
                "/usr/lib64/libvulkan.so",
                "/usr/lib64/libvulkan.so.1",
            ];

            for path in &vulkan_paths {
                if Path::new(path).exists() {
                    debug!("Found Vulkan library at {}", path);
                    // TODO: Query Vulkan device memory via vulkaninfo or ash crate
                    // For now, return 0 and let ModelSelector assume minimum viable
                    return Some(0);
                }
            }
        }

        #[cfg(target_os = "windows")]
        {
            let vulkan_paths = [
                "C:\\Windows\\System32\\vulkan-1.dll",
            ];

            for path in &vulkan_paths {
                if Path::new(path).exists() {
                    debug!("Found Vulkan library at {}", path);
                    return Some(0);
                }
            }
        }

        #[cfg(target_os = "macos")]
        {
            // MoltenVK provides Vulkan on macOS
            // Check for MoltenVK or Vulkan loader
            let vulkan_paths = [
                "/usr/local/lib/libvulkan.dylib",
                "/usr/local/lib/libvulkan.1.dylib",
                "/opt/homebrew/lib/libvulkan.dylib",
            ];

            for path in &vulkan_paths {
                if Path::new(path).exists() {
                    debug!("Found Vulkan library at {}", path);
                    return Some(0);
                }
            }
        }

        None
    }

    /// Detect system RAM.
    #[cfg(target_os = "linux")]
    fn detect_system_ram() -> u64 {
        // Read from /proc/meminfo
        if let Ok(contents) = std::fs::read_to_string("/proc/meminfo") {
            for line in contents.lines() {
                if line.starts_with("MemTotal:") {
                    // Format: "MemTotal:       16384000 kB"
                    let parts: Vec<&str> = line.split_whitespace().collect();
                    if parts.len() >= 2 {
                        if let Ok(kb) = parts[1].parse::<u64>() {
                            return kb * 1024;
                        }
                    }
                }
            }
        }

        // Fallback to sysinfo syscall
        Self::detect_system_ram_sysinfo()
    }

    #[cfg(target_os = "linux")]
    fn detect_system_ram_sysinfo() -> u64 {
        let mut info: libc::sysinfo = unsafe { std::mem::zeroed() };
        let result = unsafe { libc::sysinfo(&mut info) };
        if result == 0 {
            info.totalram * info.mem_unit as u64
        } else {
            warn!("sysinfo() failed, returning 0 for RAM");
            0
        }
    }

    #[cfg(target_os = "macos")]
    fn detect_system_ram() -> u64 {
        // Use sysctl hw.memsize
        let output = std::process::Command::new("sysctl")
            .args(["-n", "hw.memsize"])
            .output();

        match output {
            Ok(output) if output.status.success() => {
                let stdout = String::from_utf8_lossy(&output.stdout);
                stdout.trim().parse().unwrap_or(0)
            }
            _ => {
                warn!("Failed to query hw.memsize via sysctl");
                0
            }
        }
    }

    #[cfg(target_os = "windows")]
    fn detect_system_ram() -> u64 {
        // Use GetPhysicallyInstalledSystemMemory or GlobalMemoryStatusEx
        // For simplicity, shell out to wmic
        let output = std::process::Command::new("wmic")
            .args(["computersystem", "get", "TotalPhysicalMemory", "/value"])
            .output();

        match output {
            Ok(output) if output.status.success() => {
                let stdout = String::from_utf8_lossy(&output.stdout);
                for line in stdout.lines() {
                    if line.starts_with("TotalPhysicalMemory=") {
                        if let Some(value) = line.strip_prefix("TotalPhysicalMemory=") {
                            return value.trim().parse().unwrap_or(0);
                        }
                    }
                }
                0
            }
            _ => {
                warn!("Failed to query RAM via wmic");
                0
            }
        }
    }

    #[cfg(not(any(target_os = "linux", target_os = "macos", target_os = "windows")))]
    fn detect_system_ram() -> u64 {
        warn!("RAM detection not implemented for this platform");
        0
    }

    /// Detect CPU features (AVX2, AVX-512).
    fn detect_cpu_features() -> CpuFeatures {
        #[cfg(any(target_arch = "x86_64", target_arch = "x86"))]
        {
            CpuFeatures {
                avx2: std::arch::is_x86_feature_detected!("avx2"),
                avx512: std::arch::is_x86_feature_detected!("avx512f"),
            }
        }

        #[cfg(target_arch = "aarch64")]
        {
            // ARM doesn't have AVX, but NEON is always present on aarch64
            // Report false for AVX features since they don't apply
            CpuFeatures {
                avx2: false,
                avx512: false,
            }
        }

        #[cfg(not(any(target_arch = "x86_64", target_arch = "x86", target_arch = "aarch64")))]
        {
            warn!("CPU feature detection not implemented for this architecture");
            CpuFeatures::default()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_gpu_backend_display() {
        assert_eq!(format!("{}", GpuBackend::Cuda), "cuda");
        assert_eq!(format!("{}", GpuBackend::Metal), "metal");
        assert_eq!(format!("{}", GpuBackend::Vulkan), "vulkan");
        assert_eq!(format!("{}", GpuBackend::None), "cpu");
    }

    #[test]
    fn test_hardware_profile_default() {
        let profile = HardwareProfile::default();
        assert_eq!(profile.gpu_backend, GpuBackend::None);
        assert_eq!(profile.vram_bytes, 0);
        assert_eq!(profile.ram_bytes, 0);
        assert!(!profile.cpu_features.avx2);
        assert!(!profile.cpu_features.avx512);
    }

    #[test]
    fn test_detect_returns_profile() {
        // This test verifies that detect() runs without panicking
        // and returns a valid profile. The actual values depend on
        // the hardware running the test.
        let profile = HardwareDetector::detect();

        // RAM should be detected on any modern system
        // (unless running in a very restricted environment)
        // We don't assert on specific values since they're hardware-dependent
        println!("Detected profile: {:?}", profile);
    }

    #[test]
    fn test_cpu_features_detected() {
        let features = HardwareDetector::detect_cpu_features();
        // On x86_64, AVX2 should be common on modern CPUs (2013+)
        // We can't assert it's true since older CPUs exist, but we can
        // verify the detection runs without error
        println!("CPU features: avx2={}, avx512={}", features.avx2, features.avx512);
    }

    #[test]
    fn test_system_ram_detection() {
        let ram = HardwareDetector::detect_system_ram();
        // Any modern system should have at least 1GB RAM
        // If detection fails, it returns 0
        println!("System RAM: {} bytes ({:.1} GB)", ram, ram as f64 / 1e9);
        // On CI/test systems, RAM should be detectable
        #[cfg(any(target_os = "linux", target_os = "macos"))]
        assert!(ram > 0, "Expected RAM to be detected on Linux/macOS");
    }
}
