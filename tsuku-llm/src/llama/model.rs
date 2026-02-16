//! Safe wrapper for llama_model.

use std::ffi::CString;
use std::path::Path;
use std::ptr::NonNull;

use super::bindings::{
    llama_model, llama_model_free, llama_model_load_from_file, llama_model_n_ctx_train,
    llama_vocab, llama_vocab_n_tokens,
};
use super::error::{LlamaError, Result};
use super::params::ModelParams;
use super::{backend_init, bindings};

/// A loaded llama.cpp model.
///
/// This struct owns the model and will free it when dropped.
/// Create contexts from this model using `LlamaContext::new()`.
pub struct LlamaModel {
    ptr: NonNull<llama_model>,
}

// SAFETY: LlamaModel is Send because llama_model operations are thread-safe
// when not accessing the same model concurrently (which we prevent with Arc).
unsafe impl Send for LlamaModel {}

// SAFETY: LlamaModel is Sync because we only allow immutable access to the model
// through shared references.
unsafe impl Sync for LlamaModel {}

impl LlamaModel {
    /// Load a model from a GGUF file.
    ///
    /// # Arguments
    ///
    /// * `path` - Path to the GGUF model file
    /// * `params` - Model loading parameters
    ///
    /// # Returns
    ///
    /// A loaded model ready for creating contexts.
    ///
    /// # Errors
    ///
    /// Returns an error if:
    /// - The path contains invalid characters
    /// - The file cannot be read
    /// - The file is not a valid GGUF model
    pub fn load_from_file(path: &Path, params: ModelParams) -> Result<Self> {
        // Initialize backend if not already done
        backend_init();

        // Convert path to C string
        let path_str = path
            .to_str()
            .ok_or(LlamaError::InvalidPathEncoding)?;
        let c_path = CString::new(path_str)?;

        // Load the model
        let raw_params = params.into_raw();
        let ptr = unsafe { llama_model_load_from_file(c_path.as_ptr(), raw_params) };

        // Check for failure
        let ptr = NonNull::new(ptr).ok_or_else(|| LlamaError::ModelLoad {
            path: path_str.to_string(),
            reason: "llama_model_load_from_file returned null".to_string(),
        })?;

        tracing::info!("Loaded model from {}", path_str);
        Ok(Self { ptr })
    }

    /// Get the training context size of this model.
    pub fn n_ctx_train(&self) -> u32 {
        unsafe { llama_model_n_ctx_train(self.ptr.as_ptr()) as u32 }
    }

    /// Get the vocabulary size of this model.
    pub fn n_vocab(&self) -> u32 {
        let vocab = self.vocab();
        unsafe { llama_vocab_n_tokens(vocab) as u32 }
    }

    /// Get the vocabulary for this model.
    ///
    /// This is used by grammar samplers to access vocabulary data.
    pub fn vocab(&self) -> *const llama_vocab {
        unsafe { bindings::llama_model_get_vocab(self.ptr.as_ptr()) }
    }

    /// Get the raw model pointer.
    ///
    /// # Safety
    ///
    /// The pointer is valid as long as this LlamaModel exists.
    pub(crate) fn as_ptr(&self) -> *mut llama_model {
        self.ptr.as_ptr()
    }
}

impl Drop for LlamaModel {
    fn drop(&mut self) {
        tracing::debug!("Freeing llama model");
        unsafe {
            llama_model_free(self.ptr.as_ptr());
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_model_params_default() {
        let params = ModelParams::default();
        // Just verify it doesn't panic
        let _ = params.into_raw();
    }

    #[test]
    fn test_model_params_for_gpu() {
        let params = ModelParams::for_gpu();
        assert_eq!(params.n_gpu_layers, -1);
    }

    #[test]
    fn test_model_params_for_cpu() {
        let params = ModelParams::for_cpu();
        assert_eq!(params.n_gpu_layers, 0);
    }

    #[test]
    fn test_load_nonexistent_file() {
        let result = LlamaModel::load_from_file(
            Path::new("/nonexistent/path/to/model.gguf"),
            ModelParams::default(),
        );
        assert!(result.is_err());
    }
}
