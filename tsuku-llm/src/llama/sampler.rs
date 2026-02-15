//! Token sampling utilities.

use super::bindings::llama_token;

/// Token sampler for selecting the next token from logits.
pub struct Sampler {
    /// Temperature for sampling (0 = greedy, higher = more random).
    pub temperature: f32,
}

impl Default for Sampler {
    fn default() -> Self {
        Self { temperature: 0.0 } // Greedy by default for deterministic output
    }
}

impl Sampler {
    /// Create a greedy sampler (always picks the highest probability token).
    pub fn greedy() -> Self {
        Self { temperature: 0.0 }
    }

    /// Create a sampler with the given temperature.
    pub fn with_temperature(temperature: f32) -> Self {
        Self { temperature }
    }

    /// Sample the next token from logits.
    ///
    /// # Arguments
    ///
    /// * `logits` - Logits from the model (one per vocabulary token)
    ///
    /// # Returns
    ///
    /// The selected token ID.
    pub fn sample(&self, logits: &[f32]) -> llama_token {
        if self.temperature <= 0.0 {
            // Greedy: pick the token with highest logit
            self.sample_greedy(logits)
        } else {
            // Temperature sampling
            self.sample_temperature(logits)
        }
    }

    /// Greedy sampling: pick the token with highest logit.
    fn sample_greedy(&self, logits: &[f32]) -> llama_token {
        logits
            .iter()
            .enumerate()
            .max_by(|(_, a), (_, b)| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal))
            .map(|(idx, _)| idx as llama_token)
            .unwrap_or(0)
    }

    /// Temperature sampling with softmax.
    fn sample_temperature(&self, logits: &[f32]) -> llama_token {
        // Apply temperature
        let scaled: Vec<f32> = logits.iter().map(|x| x / self.temperature).collect();

        // Softmax for probabilities
        let max_logit = scaled.iter().cloned().fold(f32::NEG_INFINITY, f32::max);
        let exp_sum: f32 = scaled.iter().map(|x| (x - max_logit).exp()).sum();
        let probs: Vec<f32> = scaled.iter().map(|x| (x - max_logit).exp() / exp_sum).collect();

        // Sample from distribution
        let random: f32 = rand_simple();
        let mut cumulative = 0.0;
        for (idx, &prob) in probs.iter().enumerate() {
            cumulative += prob;
            if random < cumulative {
                return idx as llama_token;
            }
        }

        // Fallback to last token
        (probs.len() - 1) as llama_token
    }
}

/// Simple random number generator (0.0 to 1.0).
///
/// Uses a basic linear congruential generator.
/// For production, consider using a proper RNG crate.
fn rand_simple() -> f32 {
    use std::time::{SystemTime, UNIX_EPOCH};

    static mut SEED: u64 = 0;

    unsafe {
        if SEED == 0 {
            SEED = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_nanos() as u64;
        }
        // LCG parameters from Numerical Recipes
        SEED = SEED.wrapping_mul(6364136223846793005).wrapping_add(1);
        (SEED >> 33) as f32 / (1u64 << 31) as f32
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_greedy_sampling() {
        let sampler = Sampler::greedy();
        let logits = vec![1.0, 5.0, 2.0, 3.0];
        let token = sampler.sample(&logits);
        assert_eq!(token, 1); // Index of highest value (5.0)
    }

    #[test]
    fn test_greedy_sampling_negative_logits() {
        let sampler = Sampler::greedy();
        let logits = vec![-1.0, -5.0, -0.5, -3.0];
        let token = sampler.sample(&logits);
        assert_eq!(token, 2); // Index of highest value (-0.5)
    }

    #[test]
    fn test_temperature_sampling_exists() {
        let sampler = Sampler::with_temperature(1.0);
        let logits = vec![1.0, 2.0, 3.0];
        let _ = sampler.sample(&logits); // Just verify it doesn't panic
    }
}
