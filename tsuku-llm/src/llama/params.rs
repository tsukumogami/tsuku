//! Parameter structs for model and context configuration.

use super::bindings::{
    llama_context_default_params, llama_context_params, llama_model_default_params,
    llama_model_params,
};

/// Parameters for loading a model.
#[derive(Debug, Clone)]
pub struct ModelParams {
    /// Number of layers to offload to GPU (-1 = all, 0 = none).
    pub n_gpu_layers: i32,

    /// Use memory mapping for model loading.
    pub use_mmap: bool,

    /// Use memory locking to prevent swapping.
    pub use_mlock: bool,
}

impl Default for ModelParams {
    fn default() -> Self {
        let defaults = unsafe { llama_model_default_params() };
        Self {
            n_gpu_layers: defaults.n_gpu_layers,
            use_mmap: defaults.use_mmap,
            use_mlock: defaults.use_mlock,
        }
    }
}

impl ModelParams {
    /// Create model params for GPU inference (offloads all layers).
    pub fn for_gpu() -> Self {
        Self {
            n_gpu_layers: -1, // Offload all layers
            use_mmap: true,
            use_mlock: false,
        }
    }

    /// Convert to raw llama.cpp params.
    pub(crate) fn into_raw(self) -> llama_model_params {
        let mut params = unsafe { llama_model_default_params() };
        params.n_gpu_layers = self.n_gpu_layers;
        params.use_mmap = self.use_mmap;
        params.use_mlock = self.use_mlock;
        params
    }
}

/// Parameters for creating a context.
#[derive(Debug, Clone)]
pub struct ContextParams {
    /// Context size (number of tokens).
    pub n_ctx: u32,

    /// Batch size for prompt processing.
    pub n_batch: u32,

    /// Number of threads for generation.
    pub n_threads: i32,

    /// Number of threads for batch processing.
    pub n_threads_batch: i32,

    /// Enable embeddings mode.
    pub embeddings: bool,
}

impl Default for ContextParams {
    fn default() -> Self {
        let defaults = unsafe { llama_context_default_params() };
        Self {
            n_ctx: defaults.n_ctx,
            n_batch: defaults.n_batch,
            n_threads: defaults.n_threads,
            n_threads_batch: defaults.n_threads_batch,
            embeddings: defaults.embeddings,
        }
    }
}

impl ContextParams {
    /// Create context params with a specific context size.
    pub fn with_context_size(n_ctx: u32) -> Self {
        Self {
            n_ctx,
            ..Default::default()
        }
    }

    /// Convert to raw llama.cpp params.
    pub(crate) fn into_raw(self) -> llama_context_params {
        let mut params = unsafe { llama_context_default_params() };
        params.n_ctx = self.n_ctx;
        params.n_batch = self.n_batch;
        params.n_threads = self.n_threads;
        params.n_threads_batch = self.n_threads_batch;
        params.embeddings = self.embeddings;
        params
    }
}
