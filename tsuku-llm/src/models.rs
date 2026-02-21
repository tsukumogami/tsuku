//! Model download and management.
//!
//! Handles downloading GGUF model files from CDN with progress display
//! and SHA256 verification.

use std::path::{Path, PathBuf};

use futures_util::StreamExt;
use sha2::{Digest, Sha256};
use thiserror::Error;
use tokio::fs::{self, File};
use tokio::io::AsyncWriteExt;
use tracing::{debug, info, warn};

use crate::model::{ModelManifest, ModelSpec};

/// Errors that can occur during model operations.
#[derive(Error, Debug)]
pub enum ModelError {
    #[error("model '{0}' not found in manifest")]
    NotInManifest(String),

    #[error("network error: {0}")]
    Network(#[from] reqwest::Error),

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),

    #[error("checksum mismatch for '{model}': expected {expected}, got {actual}")]
    ChecksumMismatch {
        model: String,
        expected: String,
        actual: String,
    },

    #[error("download failed after {attempts} attempts: {last_error}")]
    DownloadFailed {
        attempts: u32,
        last_error: String,
    },
}

/// Progress information during download.
#[derive(Debug, Clone)]
pub struct DownloadProgress {
    /// Bytes downloaded so far
    pub bytes_downloaded: u64,
    /// Total bytes to download (from Content-Length)
    pub total_bytes: u64,
}

/// Manages model downloads and storage.
pub struct ModelManager {
    /// Directory where models are stored ($TSUKU_HOME/models/)
    models_dir: PathBuf,
    /// Model manifest with URLs and checksums
    manifest: ModelManifest,
    /// HTTP client for downloads
    client: reqwest::Client,
}

impl ModelManager {
    /// Create a new ModelManager.
    ///
    /// # Arguments
    /// * `models_dir` - Directory to store downloaded models
    pub fn new(models_dir: PathBuf) -> Self {
        Self {
            models_dir,
            manifest: ModelManifest::new(),
            client: reqwest::Client::new(),
        }
    }

    /// Create a new ModelManager with a custom manifest (for testing).
    pub fn with_manifest(models_dir: PathBuf, manifest: ModelManifest) -> Self {
        Self {
            models_dir,
            manifest,
            client: reqwest::Client::new(),
        }
    }

    /// Get the path where a model's primary file is stored.
    ///
    /// For single-file models: `{models_dir}/{model_name}.gguf`
    /// For split models: `{models_dir}/{url_filename}` where the filename
    /// is extracted from the download URL (e.g., `model-q4_k_m-00001-of-00003.gguf`).
    /// llama.cpp automatically locates subsequent split parts when given the first.
    pub fn model_path(&self, model_name: &str) -> PathBuf {
        if let Some(entry) = self.manifest.get(model_name) {
            if entry.split_count > 1 {
                if let Some(filename) = entry.download_url.rsplit('/').next() {
                    return self.models_dir.join(filename);
                }
            }
        }
        self.models_dir.join(format!("{}.gguf", model_name))
    }

    /// Get all file paths for a model (handles split files).
    fn all_model_paths(&self, model_name: &str) -> Vec<PathBuf> {
        if let Some(entry) = self.manifest.get(model_name) {
            if entry.split_count > 1 {
                return split_file_paths(&self.models_dir, &entry.download_url, entry.split_count);
            }
        }
        vec![self.models_dir.join(format!("{}.gguf", model_name))]
    }

    /// Get the path for in-progress downloads.
    fn download_dir(&self) -> PathBuf {
        self.models_dir.join(".download")
    }

    /// Get the temporary path for an in-progress download.
    fn temp_path(&self, model_name: &str) -> PathBuf {
        self.download_dir().join(format!("{}.gguf.part", model_name))
    }

    /// Check if a model exists and has valid checksum.
    pub async fn is_available(&self, model_name: &str) -> bool {
        // For split models, check all parts exist
        let paths = self.all_model_paths(model_name);
        for path in &paths {
            if !path.exists() {
                return false;
            }
        }

        // Verify checksum (skipped for models without checksums yet)
        match self.verify(model_name).await {
            Ok(valid) => valid,
            Err(e) => {
                warn!("Failed to verify model {}: {}", model_name, e);
                false
            }
        }
    }

    /// Verify the SHA256 checksum of an existing model file.
    ///
    /// For models with empty checksums (not yet computed), verification
    /// is skipped and the model is assumed valid if the file exists.
    pub async fn verify(&self, model_name: &str) -> Result<bool, ModelError> {
        let entry = self
            .manifest
            .get(model_name)
            .ok_or_else(|| ModelError::NotInManifest(model_name.to_string()))?;

        let path = self.model_path(model_name);
        if !path.exists() {
            return Ok(false);
        }

        // Skip verification when checksum is not yet computed
        if entry.sha256.is_empty() {
            debug!("Skipping checksum verification for {} (no checksum in manifest)", model_name);
            return Ok(true);
        }

        let actual_hash = compute_file_sha256(&path).await?;
        let expected_hash = &entry.sha256;

        if actual_hash != *expected_hash {
            debug!(
                "Checksum mismatch for {}: expected {}, got {}",
                model_name, expected_hash, actual_hash
            );
            return Ok(false);
        }

        Ok(true)
    }

    /// Download a model with progress callback.
    ///
    /// The progress callback is called periodically during download with
    /// the current progress information. For split models, downloads all
    /// parts sequentially.
    ///
    /// # Arguments
    /// * `model_name` - Name of the model to download
    /// * `progress` - Callback function receiving progress updates
    ///
    /// # Returns
    /// Path to the downloaded model file (first split for split models)
    pub async fn download<F>(
        &self,
        model_name: &str,
        progress: F,
    ) -> Result<PathBuf, ModelError>
    where
        F: Fn(DownloadProgress) + Send,
    {
        let entry = self
            .manifest
            .get(model_name)
            .ok_or_else(|| ModelError::NotInManifest(model_name.to_string()))?
            .clone();

        let final_path = self.model_path(model_name);

        // Check if already downloaded and valid
        if self.is_available(model_name).await {
            info!("Model {} already downloaded and verified", model_name);
            return Ok(final_path);
        }

        // Ensure directories exist
        fs::create_dir_all(&self.models_dir).await?;
        fs::create_dir_all(&self.download_dir()).await?;

        if entry.split_count > 1 {
            // Download all split files
            let urls = split_file_urls(&entry.download_url, entry.split_count);
            let paths = split_file_paths(&self.models_dir, &entry.download_url, entry.split_count);

            for (i, (url, path)) in urls.iter().zip(paths.iter()).enumerate() {
                if path.exists() {
                    info!("Split file {}/{} already exists, skipping", i + 1, entry.split_count);
                    continue;
                }

                let temp_path = self.download_dir().join(
                    format!("{}.part", path.file_name().unwrap_or_default().to_string_lossy())
                );

                info!("Downloading split {}/{} from {}", i + 1, entry.split_count, url);

                // Retry with exponential backoff
                let mut last_error = String::new();
                let mut downloaded = false;
                for attempt in 1..=3 {
                    match self
                        .download_file(url, &temp_path, &progress)
                        .await
                    {
                        Ok(()) => {
                            fs::rename(&temp_path, path).await?;
                            info!("Split {}/{} downloaded", i + 1, entry.split_count);
                            downloaded = true;
                            break;
                        }
                        Err(e) => {
                            last_error = e.to_string();
                            warn!(
                                "Download attempt {} failed for split {}/{}: {}",
                                attempt, i + 1, entry.split_count, e
                            );
                            let _ = fs::remove_file(&temp_path).await;
                            if attempt < 3 {
                                let delay = std::time::Duration::from_secs(1 << (attempt - 1));
                                tokio::time::sleep(delay).await;
                            }
                        }
                    }
                }

                if !downloaded {
                    return Err(ModelError::DownloadFailed {
                        attempts: 3,
                        last_error,
                    });
                }
            }

            Ok(final_path)
        } else {
            // Single file download with checksum verification
            let temp_path = self.temp_path(model_name);
            let url = &entry.download_url;
            let expected_sha256 = &entry.sha256;
            let expected_size = entry.size_bytes;

            // Clean up existing invalid file
            if final_path.exists() {
                warn!("Model {} exists but failed verification, re-downloading", model_name);
                fs::remove_file(&final_path).await?;
            }

            info!("Downloading model {} from {}", model_name, url);

            let mut last_error = String::new();
            for attempt in 1..=3 {
                match self
                    .download_with_verification(
                        url,
                        &temp_path,
                        expected_sha256,
                        expected_size,
                        &progress,
                    )
                    .await
                {
                    Ok(()) => {
                        fs::rename(&temp_path, &final_path).await?;
                        info!("Model {} downloaded and verified", model_name);
                        return Ok(final_path);
                    }
                    Err(e) => {
                        last_error = e.to_string();
                        warn!(
                            "Download attempt {} failed for {}: {}",
                            attempt, model_name, e
                        );
                        let _ = fs::remove_file(&temp_path).await;
                        if attempt < 3 {
                            let delay = std::time::Duration::from_secs(1 << (attempt - 1));
                            tokio::time::sleep(delay).await;
                        }
                    }
                }
            }

            Err(ModelError::DownloadFailed {
                attempts: 3,
                last_error,
            })
        }
    }

    /// Download a file with streaming SHA256 verification.
    async fn download_with_verification<F>(
        &self,
        url: &str,
        temp_path: &Path,
        expected_sha256: &str,
        expected_size: u64,
        progress: &F,
    ) -> Result<(), ModelError>
    where
        F: Fn(DownloadProgress),
    {
        let response = self.client.get(url).send().await?.error_for_status()?;

        let total_bytes = response
            .content_length()
            .unwrap_or(expected_size);

        let mut file = File::create(temp_path).await?;
        let mut hasher = Sha256::new();
        let mut bytes_downloaded: u64 = 0;

        let mut stream = response.bytes_stream();

        while let Some(chunk_result) = stream.next().await {
            let chunk = chunk_result?;

            // Write to file
            file.write_all(&chunk).await?;

            // Update hash
            hasher.update(&chunk);

            // Update progress
            bytes_downloaded += chunk.len() as u64;
            progress(DownloadProgress {
                bytes_downloaded,
                total_bytes,
            });
        }

        file.flush().await?;
        drop(file);

        // Verify checksum
        let actual_sha256 = format!("{:x}", hasher.finalize());
        if actual_sha256 != expected_sha256 {
            return Err(ModelError::ChecksumMismatch {
                model: url.to_string(),
                expected: expected_sha256.to_string(),
                actual: actual_sha256,
            });
        }

        Ok(())
    }

    /// Download a file without checksum verification (for split model parts).
    async fn download_file<F>(
        &self,
        url: &str,
        temp_path: &Path,
        progress: &F,
    ) -> Result<(), ModelError>
    where
        F: Fn(DownloadProgress),
    {
        let response = self.client.get(url).send().await?.error_for_status()?;

        let total_bytes = response.content_length().unwrap_or(0);

        let mut file = File::create(temp_path).await?;
        let mut bytes_downloaded: u64 = 0;

        let mut stream = response.bytes_stream();

        while let Some(chunk_result) = stream.next().await {
            let chunk = chunk_result?;
            file.write_all(&chunk).await?;
            bytes_downloaded += chunk.len() as u64;
            progress(DownloadProgress {
                bytes_downloaded,
                total_bytes,
            });
        }

        file.flush().await?;
        Ok(())
    }

    /// Ensure a model is available, downloading if necessary.
    ///
    /// This is the main entry point for getting a model ready for use.
    pub async fn ensure_model<F>(
        &self,
        spec: &ModelSpec,
        progress: F,
    ) -> Result<PathBuf, ModelError>
    where
        F: Fn(DownloadProgress) + Send,
    {
        if self.is_available(&spec.name).await {
            return Ok(self.model_path(&spec.name));
        }

        self.download(&spec.name, progress).await
    }

    /// Get the model manifest.
    pub fn manifest(&self) -> &ModelManifest {
        &self.manifest
    }
}

/// Generate URLs for all parts of a split GGUF model.
///
/// Given the URL for part 1 (e.g., `...q4_k_m-00001-of-00003.gguf`),
/// generates URLs for all parts by replacing the part number.
fn split_file_urls(first_url: &str, split_count: u32) -> Vec<String> {
    let total = format!("{:05}", split_count);
    (1..=split_count)
        .map(|i| {
            let from = format!("-00001-of-{}", total);
            let to = format!("-{:05}-of-{}", i, total);
            first_url.replace(&from, &to)
        })
        .collect()
}

/// Generate local file paths for all parts of a split GGUF model.
fn split_file_paths(models_dir: &Path, first_url: &str, split_count: u32) -> Vec<PathBuf> {
    split_file_urls(first_url, split_count)
        .into_iter()
        .map(|url| {
            let filename = url.rsplit('/').next().unwrap_or("unknown.gguf");
            models_dir.join(filename)
        })
        .collect()
}

/// Compute SHA256 hash of a file.
async fn compute_file_sha256(path: &Path) -> Result<String, std::io::Error> {
    use tokio::io::AsyncReadExt;

    let mut file = File::open(path).await?;
    let mut hasher = Sha256::new();
    let mut buffer = vec![0u8; 64 * 1024]; // 64KB buffer

    loop {
        let n = file.read(&mut buffer).await?;
        if n == 0 {
            break;
        }
        hasher.update(&buffer[..n]);
    }

    Ok(format!("{:x}", hasher.finalize()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;
    use crate::model::{Backend, ModelEntry};

    fn test_manifest() -> ModelManifest {
        let mut models = HashMap::new();
        models.insert(
            "test-model".to_string(),
            ModelEntry {
                quantization: "q4_k_m".to_string(),
                size_bytes: 1000,
                sha256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855".to_string(), // SHA256 of empty file
                download_url: "https://example.com/test-model.gguf".to_string(),
                split_count: 1,
                supported_backends: vec![Backend::Cuda],
            },
        );
        ModelManifest { models }
    }

    #[tokio::test]
    async fn test_model_path() {
        let manager = ModelManager::new(PathBuf::from("/tmp/models"));
        assert_eq!(
            manager.model_path("qwen2.5-7b-instruct-q4"),
            PathBuf::from("/tmp/models/qwen2.5-7b-instruct-q4_k_m-00001-of-00002.gguf")
        );
    }

    #[tokio::test]
    async fn test_temp_path() {
        let manager = ModelManager::new(PathBuf::from("/tmp/models"));
        assert_eq!(
            manager.temp_path("qwen2.5-3b-instruct-q4"),
            PathBuf::from("/tmp/models/.download/qwen2.5-3b-instruct-q4.gguf.part")
        );
    }

    #[tokio::test]
    async fn test_not_available_when_missing() {
        let temp_dir = tempfile::tempdir().unwrap();
        let manager = ModelManager::with_manifest(temp_dir.path().to_path_buf(), test_manifest());
        assert!(!manager.is_available("test-model").await);
    }

    #[tokio::test]
    async fn test_verify_returns_false_when_missing() {
        let temp_dir = tempfile::tempdir().unwrap();
        let manager = ModelManager::with_manifest(temp_dir.path().to_path_buf(), test_manifest());
        assert!(!manager.verify("test-model").await.unwrap());
    }

    #[tokio::test]
    async fn test_verify_valid_file() {
        let temp_dir = tempfile::tempdir().unwrap();
        let manager = ModelManager::with_manifest(temp_dir.path().to_path_buf(), test_manifest());

        // Create an empty file (matches the SHA256 in test_manifest)
        let model_path = manager.model_path("test-model");
        fs::write(&model_path, b"").await.unwrap();

        assert!(manager.verify("test-model").await.unwrap());
    }

    #[tokio::test]
    async fn test_verify_invalid_file() {
        let temp_dir = tempfile::tempdir().unwrap();
        let manager = ModelManager::with_manifest(temp_dir.path().to_path_buf(), test_manifest());

        // Create a non-empty file (won't match empty file hash)
        let model_path = manager.model_path("test-model");
        fs::write(&model_path, b"some content").await.unwrap();

        assert!(!manager.verify("test-model").await.unwrap());
    }

    #[tokio::test]
    async fn test_verify_unknown_model() {
        let temp_dir = tempfile::tempdir().unwrap();
        let manager = ModelManager::with_manifest(temp_dir.path().to_path_buf(), test_manifest());

        let result = manager.verify("unknown-model").await;
        assert!(matches!(result, Err(ModelError::NotInManifest(_))));
    }

    #[tokio::test]
    async fn test_compute_file_sha256() {
        let temp_dir = tempfile::tempdir().unwrap();
        let file_path = temp_dir.path().join("test.txt");

        // Write known content
        fs::write(&file_path, b"hello world").await.unwrap();

        let hash = compute_file_sha256(&file_path).await.unwrap();
        // SHA256 of "hello world"
        assert_eq!(
            hash,
            "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
        );
    }

    #[tokio::test]
    async fn test_download_error_not_in_manifest() {
        let temp_dir = tempfile::tempdir().unwrap();
        let manager = ModelManager::with_manifest(temp_dir.path().to_path_buf(), test_manifest());

        let result = manager.download("unknown-model", |_| {}).await;
        assert!(matches!(result, Err(ModelError::NotInManifest(_))));
    }

    #[tokio::test]
    async fn test_progress_callback_receives_updates() {
        use std::sync::atomic::{AtomicU64, Ordering};
        use std::sync::Arc;

        let temp_dir = tempfile::tempdir().unwrap();
        let manager = ModelManager::with_manifest(temp_dir.path().to_path_buf(), test_manifest());

        // Pre-create a valid model file so download returns early
        let model_path = manager.model_path("test-model");
        fs::write(&model_path, b"").await.unwrap();

        let progress_count = Arc::new(AtomicU64::new(0));
        let progress_count_clone = Arc::clone(&progress_count);

        let result = manager.download("test-model", move |_progress| {
            progress_count_clone.fetch_add(1, Ordering::SeqCst);
        }).await;

        assert!(result.is_ok());
        // Since file already exists and is valid, no progress updates should happen
        assert_eq!(progress_count.load(Ordering::SeqCst), 0);
    }

    /// Integration test that performs an actual HTTP download.
    /// Uses a small, stable file from httpbin.org.
    #[tokio::test]
    async fn test_real_http_download_with_verification() {
        use std::sync::atomic::{AtomicU64, Ordering};
        use std::sync::Arc;

        let temp_dir = tempfile::tempdir().unwrap();

        // Create a manifest with a real downloadable file
        // httpbin.org/bytes/1024 returns 1024 random bytes, but we need deterministic content
        // Use httpbin.org/base64/aGVsbG8gd29ybGQ= which decodes to "hello world" (11 bytes)
        // SHA256 of "hello world" = b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9
        let mut models = HashMap::new();
        models.insert(
            "test-download".to_string(),
            ModelEntry {
                quantization: "test".to_string(),
                size_bytes: 11,
                sha256: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9".to_string(),
                download_url: "https://httpbin.org/base64/aGVsbG8gd29ybGQ=".to_string(),
                split_count: 1,
                supported_backends: vec![Backend::Cuda],
            },
        );
        let manifest = ModelManifest { models };

        let manager = ModelManager::with_manifest(temp_dir.path().to_path_buf(), manifest);

        // Track progress updates
        let bytes_received = Arc::new(AtomicU64::new(0));
        let bytes_received_clone = Arc::clone(&bytes_received);
        let progress_calls = Arc::new(AtomicU64::new(0));
        let progress_calls_clone = Arc::clone(&progress_calls);

        let result = manager.download("test-download", move |progress| {
            bytes_received_clone.store(progress.bytes_downloaded, Ordering::SeqCst);
            progress_calls_clone.fetch_add(1, Ordering::SeqCst);
        }).await;

        // Should succeed
        assert!(result.is_ok(), "Download failed: {:?}", result.err());
        let path = result.unwrap();

        // File should exist
        assert!(path.exists(), "Downloaded file doesn't exist");

        // Content should be correct
        let content = fs::read_to_string(&path).await.unwrap();
        assert_eq!(content, "hello world");

        // Progress callback should have been called
        assert!(progress_calls.load(Ordering::SeqCst) > 0, "Progress callback was never called");
        assert_eq!(bytes_received.load(Ordering::SeqCst), 11, "Final bytes should be 11");

        // Verification should pass
        assert!(manager.verify("test-download").await.unwrap());
    }

    /// Test that checksum mismatch is detected during download.
    #[tokio::test]
    async fn test_real_http_download_checksum_mismatch() {
        let temp_dir = tempfile::tempdir().unwrap();

        // Use the same URL but wrong checksum
        let mut models = HashMap::new();
        models.insert(
            "bad-checksum".to_string(),
            ModelEntry {
                quantization: "test".to_string(),
                size_bytes: 11,
                sha256: "0000000000000000000000000000000000000000000000000000000000000000".to_string(),
                download_url: "https://httpbin.org/base64/aGVsbG8gd29ybGQ=".to_string(),
                split_count: 1,
                supported_backends: vec![Backend::Cuda],
            },
        );
        let manifest = ModelManifest { models };

        let manager = ModelManager::with_manifest(temp_dir.path().to_path_buf(), manifest);

        let result = manager.download("bad-checksum", |_| {}).await;

        // Should fail with checksum mismatch after retries
        assert!(matches!(result, Err(ModelError::DownloadFailed { .. })));
    }

    #[test]
    fn test_split_file_urls() {
        let first_url = "https://example.com/model-q4_k_m-00001-of-00003.gguf";
        let urls = split_file_urls(first_url, 3);
        assert_eq!(urls.len(), 3);
        assert_eq!(urls[0], "https://example.com/model-q4_k_m-00001-of-00003.gguf");
        assert_eq!(urls[1], "https://example.com/model-q4_k_m-00002-of-00003.gguf");
        assert_eq!(urls[2], "https://example.com/model-q4_k_m-00003-of-00003.gguf");
    }

    #[test]
    fn test_split_file_paths() {
        let first_url = "https://example.com/model-q4_k_m-00001-of-00003.gguf";
        let paths = split_file_paths(Path::new("/tmp/models"), first_url, 3);
        assert_eq!(paths.len(), 3);
        assert_eq!(paths[0], PathBuf::from("/tmp/models/model-q4_k_m-00001-of-00003.gguf"));
        assert_eq!(paths[1], PathBuf::from("/tmp/models/model-q4_k_m-00002-of-00003.gguf"));
        assert_eq!(paths[2], PathBuf::from("/tmp/models/model-q4_k_m-00003-of-00003.gguf"));
    }

    #[tokio::test]
    async fn test_model_path_split_model() {
        let mut models = HashMap::new();
        models.insert(
            "split-model".to_string(),
            ModelEntry {
                quantization: "q4_k_m".to_string(),
                size_bytes: 9000,
                sha256: "".to_string(),
                download_url: "https://example.com/split-model-q4_k_m-00001-of-00003.gguf".to_string(),
                split_count: 3,
                supported_backends: vec![Backend::Cuda],
            },
        );
        let manifest = ModelManifest { models };
        let manager = ModelManager::with_manifest(PathBuf::from("/tmp/models"), manifest);

        // Should return path to first split file
        assert_eq!(
            manager.model_path("split-model"),
            PathBuf::from("/tmp/models/split-model-q4_k_m-00001-of-00003.gguf")
        );
    }

    #[tokio::test]
    async fn test_verify_skips_empty_checksum() {
        let temp_dir = tempfile::tempdir().unwrap();
        let mut models = HashMap::new();
        models.insert(
            "no-checksum".to_string(),
            ModelEntry {
                quantization: "q4_k_m".to_string(),
                size_bytes: 100,
                sha256: "".to_string(),
                download_url: "https://example.com/no-checksum.gguf".to_string(),
                split_count: 1,
                supported_backends: vec![Backend::Cuda],
            },
        );
        let manifest = ModelManifest { models };
        let manager = ModelManager::with_manifest(temp_dir.path().to_path_buf(), manifest);

        // Create a file
        let model_path = manager.model_path("no-checksum");
        fs::write(&model_path, b"anything").await.unwrap();

        // Should return true despite no checksum to verify
        assert!(manager.verify("no-checksum").await.unwrap());
    }
}
