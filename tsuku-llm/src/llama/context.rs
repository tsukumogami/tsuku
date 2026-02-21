//! Safe wrapper for llama_context.

use std::ptr::NonNull;
use std::sync::Arc;

use super::bindings::{
    llama_batch_free, llama_batch_init, llama_context, llama_decode, llama_free,
    llama_get_logits_ith, llama_get_memory, llama_memory_clear, llama_n_ctx,
    llama_new_context_with_model, llama_token_to_piece, llama_tokenize,
};
use super::error::{LlamaError, Result};
use super::model::LlamaModel;
use super::params::ContextParams;

/// A llama.cpp inference context.
///
/// This struct holds a reference to the model it was created from,
/// ensuring the model outlives the context. The context is not thread-safe;
/// only one thread should use it at a time.
pub struct LlamaContext {
    ptr: NonNull<llama_context>,
    _model: Arc<LlamaModel>, // Prevent model from being freed while context exists
}

// SAFETY: LlamaContext is Send because we hold ownership and ensure single-threaded access.
// Note: llama_context is NOT thread-safe, so Sync is not implemented.
unsafe impl Send for LlamaContext {}

impl LlamaContext {
    /// Create a new context from a model.
    ///
    /// # Arguments
    ///
    /// * `model` - The model to create the context from (wrapped in Arc)
    /// * `params` - Context parameters
    ///
    /// # Errors
    ///
    /// Returns an error if context creation fails (usually due to memory).
    pub fn new(model: Arc<LlamaModel>, params: ContextParams) -> Result<Self> {
        let raw_params = params.into_raw();
        let ptr = unsafe { llama_new_context_with_model(model.as_ptr(), raw_params) };

        let ptr = NonNull::new(ptr).ok_or_else(|| {
            LlamaError::ContextCreation("llama_new_context_with_model returned null".to_string())
        })?;

        tracing::debug!("Created llama context");
        Ok(Self { ptr, _model: model })
    }

    /// Get the context size (number of tokens).
    pub fn n_ctx(&self) -> u32 {
        unsafe { llama_n_ctx(self.ptr.as_ptr()) }
    }

    /// Clear the KV cache.
    ///
    /// Call this between independent generations to reset the context.
    pub fn clear_kv_cache(&mut self) {
        unsafe {
            let memory = llama_get_memory(self.ptr.as_ptr());
            llama_memory_clear(memory, true);
        }
        tracing::debug!("Cleared KV cache");
    }

    /// Tokenize a string.
    ///
    /// # Arguments
    ///
    /// * `text` - The text to tokenize
    /// * `add_special` - Whether to add special tokens (BOS/EOS)
    /// * `parse_special` - Whether to parse special tokens in the text
    ///
    /// # Returns
    ///
    /// A vector of token IDs.
    pub fn tokenize(&self, text: &str, add_special: bool, parse_special: bool) -> Result<Vec<i32>> {
        let vocab = self._model.vocab();

        // First call with null buffer to get the required token count.
        // llama_tokenize returns -n_tokens when buffer is too small (or null/0).
        // We negate this to get the actual count needed.
        // Special case: i32::MIN indicates tokenization overflow.
        let n_tokens_result = unsafe {
            llama_tokenize(
                vocab,
                text.as_ptr() as *const i8,
                text.len() as i32,
                std::ptr::null_mut(),
                0,
                add_special,
                parse_special,
            )
        };

        // Check for overflow error (result too large for int32)
        if n_tokens_result == i32::MIN {
            return Err(LlamaError::Tokenization(
                "tokenization result exceeds int32 limit".to_string(),
            ));
        }

        // The return value is negative (the negation of the required size).
        // Negate it to get the actual required buffer size.
        let n_tokens = -n_tokens_result;

        if n_tokens <= 0 {
            // Empty tokenization result
            return Ok(Vec::new());
        }

        // Allocate buffer and tokenize
        let mut tokens = vec![0i32; n_tokens as usize];
        let actual = unsafe {
            llama_tokenize(
                vocab,
                text.as_ptr() as *const i8,
                text.len() as i32,
                tokens.as_mut_ptr(),
                tokens.len() as i32,
                add_special,
                parse_special,
            )
        };

        if actual < 0 {
            return Err(LlamaError::Tokenization(
                "llama_tokenize failed on second call".to_string(),
            ));
        }

        tokens.truncate(actual as usize);
        Ok(tokens)
    }

    /// Decode a batch of tokens.
    ///
    /// # Arguments
    ///
    /// * `tokens` - Token IDs to decode
    /// * `pos` - Starting position in the context
    ///
    /// # Returns
    ///
    /// Ok(()) if successful, Err otherwise.
    pub fn decode(&mut self, tokens: &[i32], pos: i32) -> Result<()> {
        if tokens.is_empty() {
            return Ok(());
        }

        // Check context window
        let n_ctx = self.n_ctx() as usize;
        let end_pos = pos as usize + tokens.len();
        if end_pos > n_ctx {
            return Err(LlamaError::ContextWindowExceeded {
                used: end_pos,
                max: n_ctx,
            });
        }

        // Create and fill batch
        let n_tokens = tokens.len() as i32;
        let mut batch = unsafe { llama_batch_init(n_tokens, 0, 1) };

        unsafe {
            for (i, &token) in tokens.iter().enumerate() {
                let idx = i as i32;
                *batch.token.add(i) = token;
                *batch.pos.add(i) = pos + idx;
                *batch.n_seq_id.add(i) = 1;
                *(*batch.seq_id.add(i)) = 0;
                // Only compute logits for the last token
                *batch.logits.add(i) = if i == tokens.len() - 1 { 1 } else { 0 };
            }
            batch.n_tokens = n_tokens;
        }

        // Run decode
        let result = unsafe { llama_decode(self.ptr.as_ptr(), batch) };

        // Free batch
        unsafe {
            llama_batch_free(batch);
        }

        if result != 0 {
            return Err(LlamaError::Decode(format!(
                "llama_decode returned error code {}",
                result
            )));
        }

        Ok(())
    }

    /// Get logits for a specific token position in the batch.
    ///
    /// # Arguments
    ///
    /// * `idx` - Index in the batch (typically 0 for single-token decodes, or n_tokens-1)
    ///
    /// # Returns
    ///
    /// A slice of logits (one per vocabulary token).
    pub fn get_logits(&self, idx: i32) -> &[f32] {
        let n_vocab = self._model.n_vocab() as usize;
        let ptr = unsafe { llama_get_logits_ith(self.ptr.as_ptr(), idx) };
        unsafe { std::slice::from_raw_parts(ptr, n_vocab) }
    }

    /// Get the model this context was created from.
    pub fn model(&self) -> &Arc<LlamaModel> {
        &self._model
    }

    /// Convert tokens back to text (detokenization).
    ///
    /// # Arguments
    ///
    /// * `tokens` - Token IDs to convert to text
    ///
    /// # Returns
    ///
    /// The text representation of the tokens.
    pub fn detokenize(&self, tokens: &[i32]) -> Result<String> {
        let vocab = self._model.vocab();
        let mut output = String::new();

        for &token in tokens {
            // Start with a reasonable buffer size
            let mut buf = vec![0u8; 256];
            let len = unsafe {
                llama_token_to_piece(
                    vocab,
                    token,
                    buf.as_mut_ptr() as *mut i8,
                    buf.len() as i32,
                    0,     // lstrip
                    false, // special - don't render special tokens
                )
            };

            if len < 0 {
                // Negative means buffer too small, resize and retry
                let needed = (-len) as usize;
                buf.resize(needed, 0);
                let len = unsafe {
                    llama_token_to_piece(
                        vocab,
                        token,
                        buf.as_mut_ptr() as *mut i8,
                        buf.len() as i32,
                        0,
                        false,
                    )
                };
                if len < 0 {
                    return Err(LlamaError::Detokenization(format!(
                        "failed to detokenize token {}: buffer still too small",
                        token
                    )));
                }
                buf.truncate(len as usize);
            } else {
                buf.truncate(len as usize);
            }

            output.push_str(&String::from_utf8_lossy(&buf));
        }

        Ok(output)
    }

    /// Get the raw context pointer for use with samplers.
    ///
    /// # Safety
    ///
    /// The returned pointer is valid only while the context is alive.
    /// The caller must not free or mutate the context through this pointer
    /// except through llama.cpp's sampler APIs.
    pub fn as_ptr(&mut self) -> *mut llama_context {
        self.ptr.as_ptr()
    }
}

impl Drop for LlamaContext {
    fn drop(&mut self) {
        tracing::debug!("Freeing llama context");
        unsafe {
            llama_free(self.ptr.as_ptr());
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_context_params_default() {
        let params = ContextParams::default();
        // Just verify it doesn't panic
        let _ = params.into_raw();
    }

    #[test]
    fn test_context_params_with_context_size() {
        let params = ContextParams::with_context_size(4096);
        assert_eq!(params.n_ctx, 4096);
    }
}
