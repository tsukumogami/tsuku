//! Safe Rust bindings for llama.cpp.
//!
//! This module provides safe wrappers around llama.cpp's C API, ensuring:
//! - Memory safety via RAII (Drop implementations)
//! - Proper lifetime management (Context holds Arc<Model>)
//! - Clear error handling via Result types

mod context;
mod error;
mod grammar;
mod model;
mod params;
mod sampler;

pub use context::LlamaContext;
pub use model::LlamaModel;
pub use params::{ContextParams, ModelParams};
pub use sampler::Sampler;

// Re-export bindings for internal use
#[allow(non_upper_case_globals)]
#[allow(non_camel_case_types)]
#[allow(non_snake_case)]
#[allow(dead_code)]
#[allow(clippy::all)]
mod bindings {
    include!(concat!(env!("OUT_DIR"), "/bindings.rs"));
}

use bindings::{llama_backend_free, llama_backend_init};
use std::sync::Once;

static INIT: Once = Once::new();

/// Initialize the llama.cpp backend.
///
/// This function is called automatically when loading the first model,
/// but can be called explicitly if desired. It's safe to call multiple times.
pub fn backend_init() {
    INIT.call_once(|| {
        unsafe {
            llama_backend_init();
        }
        tracing::debug!("llama.cpp backend initialized");
    });
}

/// Free the llama.cpp backend.
///
/// This should be called at program exit if desired, but is not strictly necessary
/// as the OS will clean up the memory anyway.
///
/// # Safety
///
/// After calling this function, no llama.cpp functions should be called.
pub unsafe fn backend_free() {
    llama_backend_free();
    tracing::debug!("llama.cpp backend freed");
}
