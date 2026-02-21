//! Model selection based on hardware capabilities.
//!
//! Maps detected hardware profiles to appropriate model configurations,
//! balancing inference quality against resource constraints.

use std::collections::HashMap;

use crate::hardware::{GpuBackend, HardwareProfile};

/// Inference backend for model execution.
///
/// GPU acceleration is required — CPU-only inference is not supported
/// because models below 7B (the minimum for acceptable quality) are
/// too slow on CPU to be practical.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Backend {
    /// NVIDIA CUDA acceleration
    Cuda,
    /// Apple Metal acceleration
    Metal,
    /// Vulkan acceleration (AMD, Intel, NVIDIA fallback)
    Vulkan,
}

impl std::fmt::Display for Backend {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Backend::Cuda => write!(f, "cuda"),
            Backend::Metal => write!(f, "metal"),
            Backend::Vulkan => write!(f, "vulkan"),
        }
    }
}

impl std::str::FromStr for Backend {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "cuda" => Ok(Backend::Cuda),
            "metal" => Ok(Backend::Metal),
            "vulkan" => Ok(Backend::Vulkan),
            _ => Err(format!("unknown backend: {} (gpu required, cpu not supported)", s)),
        }
    }
}

/// Complete specification for a model to load.
#[derive(Debug, Clone)]
pub struct ModelSpec {
    /// Model identifier (e.g., "qwen2.5-3b-instruct-q4")
    pub name: String,
    /// Quantization level (e.g., "q4_k_m")
    pub quantization: String,
    /// Inference backend to use
    pub backend: Backend,
    /// Expected model file size in bytes (for download progress)
    pub size_bytes: u64,
    /// SHA256 checksum for verification
    pub sha256: String,
    /// CDN download URL
    pub download_url: String,
}

/// Error during model selection.
#[derive(Debug, Clone)]
pub enum SelectionError {
    /// No GPU detected — GPU acceleration is required
    NoGpuDetected,
    /// GPU doesn't have enough VRAM for the minimum model (7B)
    InsufficientVram {
        vram_gb: f64,
        minimum_gb: f64,
    },
    /// Config specifies unknown model name
    InvalidConfigModel {
        name: String,
    },
    /// Config specifies invalid or incompatible backend
    InvalidConfigBackend {
        backend: String,
        reason: String,
    },
}

impl std::fmt::Display for SelectionError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            SelectionError::NoGpuDetected => {
                write!(
                    f,
                    "no GPU detected: tsuku-llm requires a GPU with at least 8 GB VRAM"
                )
            }
            SelectionError::InsufficientVram { vram_gb, minimum_gb } => {
                write!(
                    f,
                    "insufficient VRAM: {:.1} GB available, {:.1} GB required for minimum model (7B)",
                    vram_gb, minimum_gb
                )
            }
            SelectionError::InvalidConfigModel { name } => {
                write!(f, "unknown model in config: {}", name)
            }
            SelectionError::InvalidConfigBackend { backend, reason } => {
                write!(f, "invalid backend '{}': {}", backend, reason)
            }
        }
    }
}

impl std::error::Error for SelectionError {}

/// Entry in the model manifest.
#[derive(Debug, Clone)]
pub struct ModelEntry {
    /// Quantization level
    pub quantization: String,
    /// Expected file size in bytes (total across all split files)
    pub size_bytes: u64,
    /// SHA256 checksum (of the first split file, or the single file)
    pub sha256: String,
    /// Download URL (first split file URL, or single file URL)
    pub download_url: String,
    /// Number of split GGUF files (1 = single file, >1 = split)
    pub split_count: u32,
    /// Supported backends for this model
    pub supported_backends: Vec<Backend>,
}

/// Manifest of available models.
#[derive(Debug, Clone)]
pub struct ModelManifest {
    pub models: HashMap<String, ModelEntry>,
}

impl Default for ModelManifest {
    fn default() -> Self {
        Self::new()
    }
}

impl ModelManifest {
    /// Create a new manifest with the default bundled models.
    pub fn new() -> Self {
        let mut models = HashMap::new();

        // Qwen 2.5 14B Instruct Q4 (split into 3 files, ~9 GB total)
        models.insert(
            "qwen2.5-14b-instruct-q4".to_string(),
            ModelEntry {
                quantization: "q4_k_m".to_string(),
                size_bytes: 9_147_539_680,
                sha256: "".to_string(), // TODO: compute after download validation
                download_url: "https://huggingface.co/Qwen/Qwen2.5-14B-Instruct-GGUF/resolve/main/qwen2.5-14b-instruct-q4_k_m-00001-of-00003.gguf"
                    .to_string(),
                split_count: 3,
                supported_backends: vec![Backend::Cuda, Backend::Metal, Backend::Vulkan],
            },
        );

        // Qwen 2.5 7B Instruct Q4 (split into 2 files, ~4.7 GB total)
        models.insert(
            "qwen2.5-7b-instruct-q4".to_string(),
            ModelEntry {
                quantization: "q4_k_m".to_string(),
                size_bytes: 4_940_752_032,
                sha256: "".to_string(), // TODO: compute after download validation
                download_url: "https://huggingface.co/Qwen/Qwen2.5-7B-Instruct-GGUF/resolve/main/qwen2.5-7b-instruct-q4_k_m-00001-of-00002.gguf"
                    .to_string(),
                split_count: 2,
                supported_backends: vec![Backend::Cuda, Backend::Metal, Backend::Vulkan],
            },
        );

        Self { models }
    }

    /// Get a model entry by name.
    pub fn get(&self, name: &str) -> Option<&ModelEntry> {
        self.models.get(name)
    }

    /// List all available model names.
    pub fn model_names(&self) -> Vec<&str> {
        self.models.keys().map(|s| s.as_str()).collect()
    }
}

/// Configuration overrides for model selection.
#[derive(Debug, Clone, Default)]
pub struct ModelConfig {
    /// Override automatic model selection
    pub local_model: Option<String>,
    /// Override automatic backend selection
    pub local_backend: Option<String>,
}

/// Selects appropriate models based on hardware capabilities.
pub struct ModelSelector {
    manifest: ModelManifest,
    config: ModelConfig,
}

// VRAM thresholds in bytes.
// Model VRAM usage (Q4_K_M with 24K context, approximate):
//   14B: ~12.6 GB (model 8.1 GB + KV cache 4.5 GB)
//   7B:  ~6.5 GB  (model 4.7 GB + KV cache 1.5 GB + compute 0.3 GB)
const GB: u64 = 1_000_000_000;
const VRAM_THRESHOLD_14B: u64 = 14 * GB;
const VRAM_THRESHOLD_7B: u64 = 8 * GB;
const MINIMUM_VRAM_GB: f64 = 8.0;

impl ModelSelector {
    /// Create a new selector with the default manifest.
    pub fn new() -> Self {
        Self {
            manifest: ModelManifest::new(),
            config: ModelConfig::default(),
        }
    }

    /// Create a new selector with custom config.
    pub fn with_config(config: ModelConfig) -> Self {
        Self {
            manifest: ModelManifest::new(),
            config,
        }
    }

    /// Create a new selector with custom manifest and config (for testing).
    pub fn with_manifest_and_config(manifest: ModelManifest, config: ModelConfig) -> Self {
        Self { manifest, config }
    }

    /// Select the best model for the given hardware profile.
    ///
    /// Requires a GPU with at least 8 GB VRAM. Returns an error if no
    /// GPU is detected or VRAM is insufficient for the minimum model (7B).
    pub fn select(&self, profile: &HardwareProfile) -> Result<ModelSpec, SelectionError> {
        // Check for config overrides first
        if let Some(ref model_name) = self.config.local_model {
            return self.build_spec_from_override(model_name, profile);
        }

        // Require GPU
        if profile.gpu_backend == GpuBackend::None {
            return Err(SelectionError::NoGpuDetected);
        }

        // Require minimum VRAM for 7B model
        if profile.vram_bytes < VRAM_THRESHOLD_7B {
            let vram_gb = profile.vram_bytes as f64 / GB as f64;
            return Err(SelectionError::InsufficientVram {
                vram_gb,
                minimum_gb: MINIMUM_VRAM_GB,
            });
        }

        let model_name = self.select_model_for_hardware(profile);
        let backend = self.select_backend(profile)?;

        self.build_spec(&model_name, backend)
    }

    /// Select model based on VRAM. Caller must ensure GPU is present and
    /// has at least VRAM_THRESHOLD_7B.
    fn select_model_for_hardware(&self, profile: &HardwareProfile) -> String {
        if profile.vram_bytes >= VRAM_THRESHOLD_14B {
            "qwen2.5-14b-instruct-q4".to_string()
        } else {
            "qwen2.5-7b-instruct-q4".to_string()
        }
    }

    /// Select backend based on hardware and config.
    fn select_backend(&self, profile: &HardwareProfile) -> Result<Backend, SelectionError> {
        // Check for config override
        if let Some(ref backend_str) = self.config.local_backend {
            let backend: Backend = backend_str.parse().map_err(|_| {
                SelectionError::InvalidConfigBackend {
                    backend: backend_str.clone(),
                    reason: "must be one of: cuda, metal, vulkan".to_string(),
                }
            })?;

            self.validate_backend(backend, profile)?;
            return Ok(backend);
        }

        // Auto-select based on detected GPU
        match profile.gpu_backend {
            GpuBackend::Cuda => Ok(Backend::Cuda),
            GpuBackend::Metal => Ok(Backend::Metal),
            GpuBackend::Vulkan => Ok(Backend::Vulkan),
            GpuBackend::None => Err(SelectionError::NoGpuDetected),
        }
    }

    /// Validate that a backend is available on the current hardware.
    fn validate_backend(&self, backend: Backend, profile: &HardwareProfile) -> Result<(), SelectionError> {
        let backend_str = backend.to_string();

        match backend {
            Backend::Cuda => {
                if profile.gpu_backend != GpuBackend::Cuda {
                    return Err(SelectionError::InvalidConfigBackend {
                        backend: backend_str,
                        reason: "CUDA not available on this system".to_string(),
                    });
                }
            }
            Backend::Metal => {
                if profile.gpu_backend != GpuBackend::Metal {
                    return Err(SelectionError::InvalidConfigBackend {
                        backend: backend_str,
                        reason: "Metal not available on this system".to_string(),
                    });
                }
            }
            Backend::Vulkan => {
                if profile.gpu_backend != GpuBackend::Vulkan && profile.gpu_backend != GpuBackend::Cuda {
                    return Err(SelectionError::InvalidConfigBackend {
                        backend: backend_str,
                        reason: "Vulkan not available on this system".to_string(),
                    });
                }
            }
        }

        Ok(())
    }

    /// Build a ModelSpec from a model override in config.
    fn build_spec_from_override(
        &self,
        model_name: &str,
        profile: &HardwareProfile,
    ) -> Result<ModelSpec, SelectionError> {
        let entry = self.manifest.get(model_name).ok_or_else(|| {
            SelectionError::InvalidConfigModel {
                name: model_name.to_string(),
            }
        })?;

        let backend = self.select_backend(profile)?;

        // Validate the model supports the selected backend
        if !entry.supported_backends.contains(&backend) {
            return Err(SelectionError::InvalidConfigBackend {
                backend: backend.to_string(),
                reason: format!(
                    "model '{}' does not support backend '{}'",
                    model_name, backend
                ),
            });
        }

        Ok(ModelSpec {
            name: model_name.to_string(),
            quantization: entry.quantization.clone(),
            backend,
            size_bytes: entry.size_bytes,
            sha256: entry.sha256.clone(),
            download_url: entry.download_url.clone(),
        })
    }

    /// Build a ModelSpec from auto-selected model name and backend.
    fn build_spec(&self, model_name: &str, backend: Backend) -> Result<ModelSpec, SelectionError> {
        let entry = self.manifest.get(model_name).ok_or_else(|| {
            SelectionError::InvalidConfigModel {
                name: model_name.to_string(),
            }
        })?;

        Ok(ModelSpec {
            name: model_name.to_string(),
            quantization: entry.quantization.clone(),
            backend,
            size_bytes: entry.size_bytes,
            sha256: entry.sha256.clone(),
            download_url: entry.download_url.clone(),
        })
    }

    /// Get the model manifest.
    pub fn manifest(&self) -> &ModelManifest {
        &self.manifest
    }
}

impl Default for ModelSelector {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::hardware::CpuFeatures;

    fn make_profile(gpu: GpuBackend, vram_gb: u64, ram_gb: u64) -> HardwareProfile {
        HardwareProfile {
            gpu_backend: gpu,
            vram_bytes: vram_gb * GB,
            ram_bytes: ram_gb * GB,
            cpu_features: CpuFeatures::default(),
        }
    }

    // Selection tests - GPU path

    #[test]
    fn test_gpu_16gb_vram_selects_14b() {
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::Cuda, 16, 32);

        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.name, "qwen2.5-14b-instruct-q4");
        assert_eq!(spec.backend, Backend::Cuda);
    }

    #[test]
    fn test_gpu_8gb_vram_selects_7b() {
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::Cuda, 8, 16);

        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.name, "qwen2.5-7b-instruct-q4");
        assert_eq!(spec.backend, Backend::Cuda);
    }

    #[test]
    fn test_gpu_low_vram_returns_error() {
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::Vulkan, 4, 16);

        let result = selector.select(&profile);
        assert!(matches!(result, Err(SelectionError::InsufficientVram { .. })));
    }

    #[test]
    fn test_no_gpu_returns_error() {
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::None, 0, 32);

        let result = selector.select(&profile);
        assert!(matches!(result, Err(SelectionError::NoGpuDetected)));
    }

    // Config override tests

    #[test]
    fn test_config_model_override() {
        let config = ModelConfig {
            local_model: Some("qwen2.5-7b-instruct-q4".to_string()),
            local_backend: None,
        };
        let selector = ModelSelector::with_config(config);
        let profile = make_profile(GpuBackend::Cuda, 16, 32);

        // Should use config model despite having plenty of VRAM for 14B
        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.name, "qwen2.5-7b-instruct-q4");
    }

    #[test]
    fn test_invalid_config_model_returns_error() {
        let config = ModelConfig {
            local_model: Some("nonexistent-model".to_string()),
            local_backend: None,
        };
        let selector = ModelSelector::with_config(config);
        let profile = make_profile(GpuBackend::Cuda, 8, 16);

        let result = selector.select(&profile);
        assert!(matches!(
            result,
            Err(SelectionError::InvalidConfigModel { .. })
        ));
    }

    #[test]
    fn test_invalid_config_backend_returns_error() {
        let config = ModelConfig {
            local_model: None,
            local_backend: Some("invalid-backend".to_string()),
        };
        let selector = ModelSelector::with_config(config);
        let profile = make_profile(GpuBackend::Cuda, 8, 16);

        let result = selector.select(&profile);
        assert!(matches!(
            result,
            Err(SelectionError::InvalidConfigBackend { .. })
        ));
    }

    #[test]
    fn test_no_gpu_with_backend_override_returns_error() {
        let config = ModelConfig {
            local_model: None,
            local_backend: Some("cuda".to_string()),
        };
        let selector = ModelSelector::with_config(config);
        let profile = make_profile(GpuBackend::None, 0, 16);

        // No GPU detected, so selection fails before backend override matters
        let result = selector.select(&profile);
        assert!(matches!(result, Err(SelectionError::NoGpuDetected)));
    }

    // Backend selection tests

    #[test]
    fn test_metal_backend_selection() {
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::Metal, 8, 16);

        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.backend, Backend::Metal);
    }

    #[test]
    fn test_backend_from_str() {
        assert_eq!("cuda".parse::<Backend>().unwrap(), Backend::Cuda);
        assert_eq!("metal".parse::<Backend>().unwrap(), Backend::Metal);
        assert_eq!("vulkan".parse::<Backend>().unwrap(), Backend::Vulkan);
        assert_eq!("CUDA".parse::<Backend>().unwrap(), Backend::Cuda); // case insensitive
        assert!("cpu".parse::<Backend>().is_err());
        assert!("invalid".parse::<Backend>().is_err());
    }

    #[test]
    fn test_model_manifest_has_all_models() {
        let manifest = ModelManifest::new();
        assert!(manifest.get("qwen2.5-14b-instruct-q4").is_some());
        assert!(manifest.get("qwen2.5-7b-instruct-q4").is_some());
        assert!(manifest.get("qwen2.5-3b-instruct-q4").is_none()); // removed: below quality floor
        assert!(manifest.get("qwen2.5-0.5b-instruct-q4").is_none()); // removed: below quality floor
    }

    #[test]
    fn test_model_spec_has_required_fields() {
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::Cuda, 8, 16);

        let spec = selector.select(&profile).unwrap();
        assert!(!spec.name.is_empty());
        assert!(!spec.quantization.is_empty());
        assert!(spec.size_bytes > 0);
        assert!(!spec.download_url.is_empty());
    }
}
