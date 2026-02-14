//! Error types for llama.cpp operations.

use std::ffi::NulError;
use thiserror::Error;

/// Errors that can occur during llama.cpp operations.
#[derive(Error, Debug)]
pub enum LlamaError {
    /// Failed to load a model from file.
    #[error("failed to load model from '{path}': {reason}")]
    ModelLoad { path: String, reason: String },

    /// Failed to create a context.
    #[error("failed to create context: {0}")]
    ContextCreation(String),

    /// Failed to tokenize input.
    #[error("tokenization failed: {0}")]
    Tokenization(String),

    /// Failed to detokenize output.
    #[error("detokenization failed: {0}")]
    Detokenization(String),

    /// Failed to decode tokens.
    #[error("decode failed: {0}")]
    Decode(String),

    /// Invalid parameter value.
    #[error("invalid parameter: {0}")]
    InvalidParam(String),

    /// Path contains invalid characters.
    #[error("path contains invalid characters (interior NUL byte)")]
    InvalidPath(#[from] NulError),

    /// Path is not valid UTF-8.
    #[error("path is not valid UTF-8")]
    InvalidPathEncoding,

    /// Out of memory.
    #[error("out of memory: {0}")]
    OutOfMemory(String),

    /// Context window exceeded.
    #[error("context window exceeded: {used} tokens used, {max} maximum")]
    ContextWindowExceeded { used: usize, max: usize },

    /// Backend not available.
    #[error("backend '{0}' not available")]
    BackendNotAvailable(String),

    /// Grammar error.
    #[error("grammar error: {0}")]
    Grammar(String),
}

/// Result type alias for llama operations.
pub type Result<T> = std::result::Result<T, LlamaError>;
