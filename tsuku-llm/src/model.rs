//! Model selection based on hardware capabilities.
//!
//! Maps detected hardware profiles to appropriate model configurations,
//! balancing inference quality against resource constraints.

use std::collections::HashMap;

use crate::hardware::{GpuBackend, HardwareProfile};

/// Inference backend for model execution.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Backend {
    /// NVIDIA CUDA acceleration
    Cuda,
    /// Apple Metal acceleration
    Metal,
    /// Vulkan acceleration (AMD, Intel, NVIDIA fallback)
    Vulkan,
    /// CPU-only inference
    Cpu,
}

impl std::fmt::Display for Backend {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Backend::Cuda => write!(f, "cuda"),
            Backend::Metal => write!(f, "metal"),
            Backend::Vulkan => write!(f, "vulkan"),
            Backend::Cpu => write!(f, "cpu"),
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
            "cpu" => Ok(Backend::Cpu),
            _ => Err(format!("unknown backend: {}", s)),
        }
    }
}

impl From<GpuBackend> for Backend {
    fn from(gpu: GpuBackend) -> Self {
        match gpu {
            GpuBackend::Cuda => Backend::Cuda,
            GpuBackend::Metal => Backend::Metal,
            GpuBackend::Vulkan => Backend::Vulkan,
            GpuBackend::None => Backend::Cpu,
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
    /// System doesn't meet minimum resource requirements
    InsufficientResources {
        ram_gb: f64,
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
            SelectionError::InsufficientResources { ram_gb, minimum_gb } => {
                write!(
                    f,
                    "insufficient resources: {:.1}GB RAM available, {:.1}GB required",
                    ram_gb, minimum_gb
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
    /// Expected file size in bytes
    pub size_bytes: u64,
    /// SHA256 checksum
    pub sha256: String,
    /// Download URL
    pub download_url: String,
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

        // Qwen 2.5 3B Instruct Q4
        models.insert(
            "qwen2.5-3b-instruct-q4".to_string(),
            ModelEntry {
                quantization: "q4_k_m".to_string(),
                size_bytes: 2_104_932_768,
                sha256: "626b4a6678b86442240e33df819e00132d3ba7dddfe1cdc4fbb18e0a9615c62d".to_string(),
                download_url: "https://huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF/resolve/main/qwen2.5-3b-instruct-q4_k_m.gguf"
                    .to_string(),
                supported_backends: vec![Backend::Cuda, Backend::Metal, Backend::Vulkan, Backend::Cpu],
            },
        );

        // Qwen 2.5 1.5B Instruct Q4
        models.insert(
            "qwen2.5-1.5b-instruct-q4".to_string(),
            ModelEntry {
                quantization: "q4_k_m".to_string(),
                size_bytes: 1_117_320_736,
                sha256: "6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e".to_string(),
                download_url: "https://huggingface.co/Qwen/Qwen2.5-1.5B-Instruct-GGUF/resolve/main/qwen2.5-1.5b-instruct-q4_k_m.gguf"
                    .to_string(),
                supported_backends: vec![Backend::Cuda, Backend::Metal, Backend::Vulkan, Backend::Cpu],
            },
        );

        // Qwen 2.5 0.5B Instruct Q4
        models.insert(
            "qwen2.5-0.5b-instruct-q4".to_string(),
            ModelEntry {
                quantization: "q4_k_m".to_string(),
                size_bytes: 491_400_032,
                sha256: "74a4da8c9fdbcd15bd1f6d01d621410d31c6fc00986f5eb687824e7b93d7a9db".to_string(),
                download_url: "https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q4_k_m.gguf"
                    .to_string(),
                supported_backends: vec![Backend::Cuda, Backend::Metal, Backend::Vulkan, Backend::Cpu],
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

// Memory thresholds in bytes
const GB: u64 = 1_000_000_000;
const MINIMUM_RAM_GB: f64 = 4.0;
const VRAM_THRESHOLD_HIGH: u64 = 8 * GB;
const VRAM_THRESHOLD_MED: u64 = 4 * GB;
const RAM_THRESHOLD_HIGH: u64 = 16 * GB;
const RAM_THRESHOLD_MED: u64 = 8 * GB;
const RAM_THRESHOLD_MIN: u64 = 4 * GB;

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
    pub fn select(&self, profile: &HardwareProfile) -> Result<ModelSpec, SelectionError> {
        // Check for config overrides first
        if let Some(ref model_name) = self.config.local_model {
            return self.build_spec_from_override(model_name, profile);
        }

        // Check minimum resources
        let ram_gb = profile.ram_bytes as f64 / GB as f64;
        if profile.ram_bytes < RAM_THRESHOLD_MIN {
            return Err(SelectionError::InsufficientResources {
                ram_gb,
                minimum_gb: MINIMUM_RAM_GB,
            });
        }

        // Apply selection table
        let model_name = self.select_model_for_hardware(profile);
        let backend = self.select_backend(profile)?;

        self.build_spec(&model_name, backend)
    }

    /// Select model based on hardware capabilities.
    fn select_model_for_hardware(&self, profile: &HardwareProfile) -> String {
        let has_gpu = profile.gpu_backend != GpuBackend::None;

        if has_gpu {
            // GPU path: select based on VRAM
            if profile.vram_bytes >= VRAM_THRESHOLD_HIGH {
                "qwen2.5-3b-instruct-q4".to_string()
            } else if profile.vram_bytes >= VRAM_THRESHOLD_MED {
                "qwen2.5-1.5b-instruct-q4".to_string()
            } else {
                "qwen2.5-0.5b-instruct-q4".to_string()
            }
        } else {
            // CPU-only path: select based on system RAM
            if profile.ram_bytes >= RAM_THRESHOLD_HIGH {
                "qwen2.5-3b-instruct-q4".to_string()
            } else if profile.ram_bytes >= RAM_THRESHOLD_MED {
                "qwen2.5-1.5b-instruct-q4".to_string()
            } else {
                "qwen2.5-0.5b-instruct-q4".to_string()
            }
        }
    }

    /// Select backend based on hardware and config.
    fn select_backend(&self, profile: &HardwareProfile) -> Result<Backend, SelectionError> {
        // Check for config override
        if let Some(ref backend_str) = self.config.local_backend {
            let backend: Backend = backend_str.parse().map_err(|_| {
                SelectionError::InvalidConfigBackend {
                    backend: backend_str.clone(),
                    reason: format!(
                        "must be one of: cuda, metal, vulkan, cpu"
                    ),
                }
            })?;

            // Validate the backend is available
            self.validate_backend(backend, profile)?;
            return Ok(backend);
        }

        // Auto-select based on detected GPU
        Ok(Backend::from(profile.gpu_backend))
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
                    // Vulkan is also available on CUDA systems as fallback
                    return Err(SelectionError::InvalidConfigBackend {
                        backend: backend_str,
                        reason: "Vulkan not available on this system".to_string(),
                    });
                }
            }
            Backend::Cpu => {
                // CPU is always available
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

    // Selection table tests

    #[test]
    fn test_gpu_high_vram_selects_3b() {
        // VRAM >= 8GB + GPU: 3B Q4
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::Cuda, 8, 16);

        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.name, "qwen2.5-3b-instruct-q4");
        assert_eq!(spec.backend, Backend::Cuda);
    }

    #[test]
    fn test_gpu_medium_vram_selects_1_5b() {
        // VRAM 4-8GB + GPU: 1.5B Q4
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::Cuda, 6, 16);

        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.name, "qwen2.5-1.5b-instruct-q4");
        assert_eq!(spec.backend, Backend::Cuda);
    }

    #[test]
    fn test_gpu_low_vram_selects_0_5b() {
        // VRAM < 4GB + GPU: 0.5B Q4
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::Vulkan, 2, 16);

        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.name, "qwen2.5-0.5b-instruct-q4");
        assert_eq!(spec.backend, Backend::Vulkan);
    }

    #[test]
    fn test_cpu_high_ram_selects_3b() {
        // CPU only, RAM >= 16GB: 3B Q4
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::None, 0, 16);

        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.name, "qwen2.5-3b-instruct-q4");
        assert_eq!(spec.backend, Backend::Cpu);
    }

    #[test]
    fn test_cpu_medium_ram_selects_1_5b() {
        // CPU only, RAM >= 8GB: 1.5B Q4
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::None, 0, 8);

        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.name, "qwen2.5-1.5b-instruct-q4");
        assert_eq!(spec.backend, Backend::Cpu);
    }

    #[test]
    fn test_cpu_low_ram_selects_0_5b() {
        // CPU only, RAM >= 4GB: 0.5B Q4
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::None, 0, 4);

        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.name, "qwen2.5-0.5b-instruct-q4");
        assert_eq!(spec.backend, Backend::Cpu);
    }

    #[test]
    fn test_insufficient_ram_returns_error() {
        // RAM < 4GB: local inference disabled
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::None, 0, 2);

        let result = selector.select(&profile);
        assert!(matches!(
            result,
            Err(SelectionError::InsufficientResources { .. })
        ));
    }

    // Config override tests

    #[test]
    fn test_config_model_override() {
        let config = ModelConfig {
            local_model: Some("qwen2.5-0.5b-instruct-q4".to_string()),
            local_backend: None,
        };
        let selector = ModelSelector::with_config(config);
        let profile = make_profile(GpuBackend::Cuda, 16, 32);

        // Should use config model despite having plenty of VRAM
        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.name, "qwen2.5-0.5b-instruct-q4");
    }

    #[test]
    fn test_config_backend_override() {
        let config = ModelConfig {
            local_model: None,
            local_backend: Some("cpu".to_string()),
        };
        let selector = ModelSelector::with_config(config);
        let profile = make_profile(GpuBackend::Cuda, 16, 32);

        // Should use CPU backend despite having CUDA
        let spec = selector.select(&profile).unwrap();
        assert_eq!(spec.backend, Backend::Cpu);
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
    fn test_incompatible_backend_returns_error() {
        let config = ModelConfig {
            local_model: None,
            local_backend: Some("cuda".to_string()),
        };
        let selector = ModelSelector::with_config(config);
        // No GPU available
        let profile = make_profile(GpuBackend::None, 0, 16);

        let result = selector.select(&profile);
        assert!(matches!(
            result,
            Err(SelectionError::InvalidConfigBackend { .. })
        ));
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
        assert_eq!("cpu".parse::<Backend>().unwrap(), Backend::Cpu);
        assert_eq!("CUDA".parse::<Backend>().unwrap(), Backend::Cuda); // case insensitive
        assert!("invalid".parse::<Backend>().is_err());
    }

    #[test]
    fn test_model_manifest_has_all_models() {
        let manifest = ModelManifest::new();
        assert!(manifest.get("qwen2.5-3b-instruct-q4").is_some());
        assert!(manifest.get("qwen2.5-1.5b-instruct-q4").is_some());
        assert!(manifest.get("qwen2.5-0.5b-instruct-q4").is_some());
    }

    #[test]
    fn test_model_spec_has_required_fields() {
        let selector = ModelSelector::new();
        let profile = make_profile(GpuBackend::Cuda, 8, 16);

        let spec = selector.select(&profile).unwrap();
        assert!(!spec.name.is_empty());
        assert!(!spec.quantization.is_empty());
        assert!(spec.size_bytes > 0);
        assert!(!spec.sha256.is_empty());
        assert!(!spec.download_url.is_empty());
    }
}
