//! tsuku-llm: Local LLM inference server for tsuku.
//!
//! This binary provides local inference capabilities via gRPC over Unix domain sockets.
//! It bundles llama.cpp and handles hardware detection, model management, and inference.

mod hardware;
mod llama;
mod model;
mod models;

use std::fs::File;
use std::os::unix::io::AsRawFd;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{bail, Context, Result};
use clap::{Parser, Subcommand};
use tokio::net::UnixListener;
use tokio::sync::{mpsc, Mutex};
use tokio_stream::wrappers::UnixListenerStream;
use tonic::{Request, Response, Status};
use tracing::{debug, error, info, warn};

use llama::{ContextParams, GrammarSampler, LlamaContext, LlamaModel, ModelParams, Sampler, json_schema_to_gbnf};

// Generated from proto/llm.proto
pub mod proto {
    tonic::include_proto!("tsuku.llm.v1");
}

use proto::inference_service_server::{InferenceService, InferenceServiceServer};
use proto::{
    CompletionRequest, CompletionResponse, ShutdownRequest, ShutdownResponse, StatusRequest,
    StatusResponse, Usage,
};

/// Grace period for in-flight requests during shutdown.
const SHUTDOWN_GRACE_PERIOD: Duration = Duration::from_secs(10);

#[derive(Parser)]
#[command(name = "tsuku-llm")]
#[command(about = "Local LLM inference server for tsuku")]
struct Cli {
    #[command(subcommand)]
    command: Option<Commands>,
}

#[derive(Subcommand)]
enum Commands {
    /// Start the inference server
    Serve {
        /// Idle timeout before automatic shutdown (e.g., "5m", "300s")
        #[arg(long, default_value = "5m", value_parser = parse_duration)]
        idle_timeout: Duration,
    },
}

/// Parse a duration string (e.g., "5m", "300s", "1h30m").
fn parse_duration(s: &str) -> Result<Duration, String> {
    // Try parsing as Go-style duration
    let s = s.trim();
    if s.is_empty() {
        return Err("empty duration".to_string());
    }

    let mut total_secs: u64 = 0;
    let mut num_start: Option<usize> = None;
    let chars: Vec<char> = s.chars().collect();
    let mut i = 0;

    while i < chars.len() {
        let c = chars[i];

        if c.is_ascii_digit() {
            if num_start.is_none() {
                num_start = Some(i);
            }
        } else if c.is_alphabetic() {
            let num_str = if let Some(start) = num_start {
                &s[start..i]
            } else {
                return Err(format!("invalid duration: missing number before '{}'", c));
            };

            let num: u64 = num_str
                .parse()
                .map_err(|_| format!("invalid number: {}", num_str))?;

            // Find the full unit suffix
            let unit_start = i;
            while i < chars.len() && chars[i].is_alphabetic() {
                i += 1;
            }
            let unit = &s[unit_start..i];

            let multiplier = match unit {
                "ns" => continue, // Nanoseconds too small, skip
                "us" | "µs" => continue, // Microseconds too small, skip
                "ms" => {
                    // Milliseconds: only add if >= 1000
                    if num >= 1000 {
                        total_secs += num / 1000;
                    }
                    num_start = None;
                    continue;
                }
                "s" => 1,
                "m" => 60,
                "h" => 3600,
                _ => return Err(format!("unknown unit: {}", unit)),
            };

            total_secs += num * multiplier;
            num_start = None;
            continue;
        }

        i += 1;
    }

    // Handle case where string ends with just a number (assume seconds)
    if let Some(start) = num_start {
        let num: u64 = s[start..]
            .parse()
            .map_err(|_| format!("invalid number: {}", &s[start..]))?;
        total_secs += num;
    }

    if total_secs == 0 {
        return Err("duration must be positive".to_string());
    }

    Ok(Duration::from_secs(total_secs))
}

/// Inference server implementation.
struct LlmServer {
    /// Loaded model name.
    model_name: String,

    /// Hardware profile detected at startup.
    hardware_profile: hardware::HardwareProfile,

    /// Signal to initiate shutdown.
    shutdown_tx: mpsc::Sender<()>,

    /// Whether the server is shutting down.
    shutting_down: Arc<AtomicBool>,

    /// Count of in-flight requests.
    in_flight: Arc<AtomicUsize>,

    /// The loaded model (shared for creating new contexts if needed).
    model: Arc<LlamaModel>,

    /// Inference context (protected by mutex since it's not Sync).
    context: Mutex<LlamaContext>,

    /// Token sampler for inference.
    sampler: Sampler,
}

impl LlmServer {
    fn new(
        model_name: String,
        hardware_profile: hardware::HardwareProfile,
        shutdown_tx: mpsc::Sender<()>,
        model: Arc<LlamaModel>,
        context: LlamaContext,
    ) -> Self {
        Self {
            model_name,
            hardware_profile,
            shutdown_tx,
            shutting_down: Arc::new(AtomicBool::new(false)),
            in_flight: Arc::new(AtomicUsize::new(0)),
            model,
            context: Mutex::new(context),
            sampler: Sampler::greedy(),
        }
    }

    fn shutting_down(&self) -> Arc<AtomicBool> {
        self.shutting_down.clone()
    }

    fn in_flight(&self) -> Arc<AtomicUsize> {
        self.in_flight.clone()
    }

    /// Build a prompt string from messages.
    ///
    /// Uses a simple chat template format compatible with most models.
    fn build_prompt(&self, messages: &[proto::Message]) -> String {
        let mut prompt = String::new();

        for msg in messages {
            let role_str = match proto::Role::try_from(msg.role) {
                Ok(proto::Role::User) => "user",
                Ok(proto::Role::Assistant) => "assistant",
                Ok(proto::Role::Tool) => "tool",
                _ => "system",
            };
            let content = &msg.content;

            // Simple chat template: <|role|>\ncontent\n
            prompt.push_str(&format!("<|{}|>\n{}\n", role_str, content));
        }

        // Add assistant prefix to prompt for completion
        prompt.push_str("<|assistant|>\n");
        prompt
    }
}

#[tonic::async_trait]
impl InferenceService for LlmServer {
    async fn complete(
        &self,
        request: Request<CompletionRequest>,
    ) -> Result<Response<CompletionResponse>, Status> {
        if self.shutting_down.load(Ordering::SeqCst) {
            return Err(Status::unavailable("Server is shutting down"));
        }

        // Track in-flight requests
        self.in_flight.fetch_add(1, Ordering::SeqCst);
        let _guard = scopeguard::guard((), |_| {
            self.in_flight.fetch_sub(1, Ordering::SeqCst);
        });

        let req = request.into_inner();
        info!(
            "Complete request: {} messages, {} tools",
            req.messages.len(),
            req.tools.len()
        );

        // Build prompt from messages
        let prompt = self.build_prompt(&req.messages);
        debug!("Built prompt: {} chars", prompt.len());

        // Acquire context lock for inference
        let mut ctx = self.context.lock().await;

        // Clear KV cache for fresh generation
        ctx.clear_kv_cache();

        // Tokenize the prompt
        let tokens = ctx.tokenize(&prompt, true, true).map_err(|e| {
            error!("Tokenization failed: {}", e);
            Status::internal(format!("Tokenization failed: {}", e))
        })?;

        let input_tokens = tokens.len();
        debug!("Tokenized {} input tokens", input_tokens);

        // Decode prompt tokens
        ctx.decode(&tokens, 0).map_err(|e| {
            error!("Decode failed: {}", e);
            Status::internal(format!("Decode failed: {}", e))
        })?;

        // Generate response tokens
        let mut output_tokens: Vec<i32> = Vec::new();
        let max_tokens = if req.max_tokens > 0 {
            req.max_tokens as usize
        } else {
            512 // Default if not specified
        };
        let mut pos = tokens.len() as i32;

        // Check if we should use grammar-constrained generation
        // Priority: json_schema field > first tool's parameters_schema
        let grammar_schema: Option<String> = if !req.json_schema.is_empty() {
            Some(req.json_schema.clone())
        } else if !req.tools.is_empty() {
            // Use the first tool's schema for grammar constraints
            let tool = &req.tools[0];
            if !tool.parameters_schema.is_empty() {
                Some(tool.parameters_schema.clone())
            } else {
                None
            }
        } else {
            None
        };

        // Create grammar sampler if schema is provided
        let mut grammar_sampler: Option<GrammarSampler> = if let Some(schema_str) = grammar_schema {
            match serde_json::from_str::<serde_json::Value>(&schema_str) {
                Ok(schema) => {
                    match json_schema_to_gbnf(&schema) {
                        Ok(grammar) => {
                            debug!("Generated GBNF grammar:\n{}", grammar);
                            match GrammarSampler::new(ctx.model().vocab(), &grammar, "root") {
                                Ok(sampler) => {
                                    info!("Using grammar-constrained generation");
                                    Some(sampler)
                                }
                                Err(e) => {
                                    warn!("Failed to create grammar sampler: {}. Falling back to unconstrained.", e);
                                    None
                                }
                            }
                        }
                        Err(e) => {
                            warn!("Failed to generate grammar from schema: {}. Falling back to unconstrained.", e);
                            None
                        }
                    }
                }
                Err(e) => {
                    warn!("Failed to parse JSON schema: {}. Falling back to unconstrained.", e);
                    None
                }
            }
        } else {
            None
        };

        // Track the batch index where logits are available.
        // After prompt decode, logits are at the last token index.
        // After single-token decodes, logits are at index 0.
        let mut logits_idx = (tokens.len() - 1) as i32;

        for _ in 0..max_tokens {
            // Sample next token (grammar-constrained or regular)
            let next_token = if let Some(ref mut gs) = grammar_sampler {
                // Grammar sampler handles everything internally
                let token = gs.sample(ctx.as_ptr(), logits_idx);
                gs.accept(token);
                token
            } else {
                // Regular sampling: get logits and sample
                let logits = ctx.get_logits(logits_idx);
                self.sampler.sample(logits)
            };

            // Check for EOS (token 0 or 2 are common EOS tokens)
            if next_token == 0 || next_token == 2 {
                break;
            }

            output_tokens.push(next_token);

            // Decode the new token
            ctx.decode(&[next_token], pos).map_err(|e| {
                error!("Decode failed during generation: {}", e);
                Status::internal(format!("Decode failed: {}", e))
            })?;

            pos += 1;
            // After single-token decode, logits are at batch index 0
            logits_idx = 0;
        }

        // Detokenize output (simplified - just return the count for now)
        // Full detokenization would use llama_token_to_piece
        let content = format!(
            "Generated {} tokens (detokenization not yet implemented)",
            output_tokens.len()
        );

        let stop_reason = if output_tokens.len() >= max_tokens {
            "max_tokens"
        } else {
            "end_turn"
        };

        let response = CompletionResponse {
            content,
            tool_calls: vec![],
            stop_reason: stop_reason.to_string(),
            usage: Some(Usage {
                input_tokens: input_tokens as i32,
                output_tokens: output_tokens.len() as i32,
            }),
        };

        Ok(Response::new(response))
    }

    async fn shutdown(
        &self,
        request: Request<ShutdownRequest>,
    ) -> Result<Response<ShutdownResponse>, Status> {
        let req = request.into_inner();
        info!("Shutdown requested (graceful={})", req.graceful);

        self.shutting_down.store(true, Ordering::SeqCst);

        // Signal the main loop to shut down
        if let Err(e) = self.shutdown_tx.send(()).await {
            warn!("Failed to send shutdown signal: {}", e);
        }

        Ok(Response::new(ShutdownResponse { accepted: true }))
    }

    async fn get_status(
        &self,
        _request: Request<StatusRequest>,
    ) -> Result<Response<StatusResponse>, Status> {
        let response = StatusResponse {
            ready: !self.shutting_down.load(Ordering::SeqCst),
            model_name: self.model_name.clone(),
            model_size_bytes: 0, // TODO: Actual model size when model is loaded
            backend: self.hardware_profile.gpu_backend.to_string(),
            available_vram_bytes: self.hardware_profile.vram_bytes as i64,
        };

        Ok(Response::new(response))
    }
}

/// Returns the path to the Unix domain socket.
fn socket_path() -> PathBuf {
    // Use TSUKU_HOME if set, otherwise default to ~/.tsuku
    let home = std::env::var("TSUKU_HOME")
        .ok()
        .map(PathBuf::from)
        .unwrap_or_else(|| {
            dirs::home_dir()
                .expect("Could not determine home directory")
                .join(".tsuku")
        });

    home.join("llm.sock")
}

/// Returns the path to the lock file.
fn lock_path() -> PathBuf {
    let mut path = socket_path();
    let mut name = path.file_name().unwrap().to_os_string();
    name.push(".lock");
    path.set_file_name(name);
    path
}

/// Tries to acquire an exclusive lock on the lock file.
/// Returns the lock file handle if successful, or an error if another process holds the lock.
fn acquire_lock(lock: &PathBuf) -> Result<File> {
    // Ensure parent directory exists
    if let Some(parent) = lock.parent() {
        std::fs::create_dir_all(parent).context("Failed to create lock directory")?;
    }

    let file = File::options()
        .create(true)
        .read(true)
        .write(true)
        .open(lock)
        .context("Failed to open lock file")?;

    // Try to acquire exclusive non-blocking lock
    let fd = file.as_raw_fd();
    let result = unsafe { libc::flock(fd, libc::LOCK_EX | libc::LOCK_NB) };

    if result != 0 {
        let err = std::io::Error::last_os_error();
        if err.kind() == std::io::ErrorKind::WouldBlock {
            bail!("Another tsuku-llm daemon is already running (lock held)");
        }
        return Err(err).context("Failed to acquire lock");
    }

    info!("Acquired exclusive lock on {:?}", lock);
    Ok(file)
}

/// Clean up socket and lock files.
fn cleanup_files(socket: &PathBuf, lock: &PathBuf) {
    if socket.exists() {
        if let Err(e) = std::fs::remove_file(socket) {
            warn!("Failed to remove socket file: {}", e);
        } else {
            info!("Removed socket file: {:?}", socket);
        }
    }

    if lock.exists() {
        if let Err(e) = std::fs::remove_file(lock) {
            warn!("Failed to remove lock file: {}", e);
        } else {
            info!("Removed lock file: {:?}", lock);
        }
    }
}

/// Wait for in-flight requests to complete with a timeout.
/// Returns true if interrupted by a second signal, false otherwise.
async fn wait_for_in_flight(
    in_flight: &Arc<AtomicUsize>,
    timeout: Duration,
    sigterm: &mut tokio::signal::unix::Signal,
) -> bool {
    let start = std::time::Instant::now();

    loop {
        let count = in_flight.load(Ordering::SeqCst);
        if count == 0 {
            info!("All in-flight requests completed");
            return false;
        }

        if start.elapsed() >= timeout {
            warn!(
                "Grace period expired with {} in-flight requests",
                count
            );
            return false;
        }

        info!(
            "Waiting for {} in-flight requests ({:.1}s remaining)",
            count,
            (timeout - start.elapsed()).as_secs_f32()
        );

        // Wait for either the poll interval or a second SIGTERM
        tokio::select! {
            _ = tokio::time::sleep(Duration::from_millis(100)) => {}
            _ = sigterm.recv() => {
                warn!("Received second SIGTERM during grace period, forcing immediate cleanup");
                return true;
            }
        }
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize logging
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive("tsuku_llm=info".parse().unwrap()),
        )
        .init();

    let cli = Cli::parse();

    // Default to serve command if none specified
    let idle_timeout = match cli.command {
        Some(Commands::Serve { idle_timeout }) => idle_timeout,
        None => Duration::from_secs(5 * 60), // Default 5 minutes
    };

    info!("Idle timeout: {:?}", idle_timeout);

    let socket = socket_path();
    let lock = lock_path();
    info!("Starting tsuku-llm server at {:?}", socket);

    // Try to acquire the lock file first
    let _lock_file = acquire_lock(&lock)?;

    // Set up SIGTERM handler EARLY - before any long-running operations like model download.
    // This ensures we can catch SIGTERM during startup and clean up properly.
    #[cfg(unix)]
    let mut sigterm = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
        .context("Failed to register SIGTERM handler")?;

    // Now that we have the lock, remove stale socket if it exists
    if socket.exists() {
        std::fs::remove_file(&socket).context("Failed to remove stale socket")?;
        info!("Removed stale socket file");
    }

    // Ensure parent directory exists
    if let Some(parent) = socket.parent() {
        std::fs::create_dir_all(parent).context("Failed to create socket directory")?;
    }

    // Create Unix listener
    let listener = UnixListener::bind(&socket).context("Failed to bind Unix socket")?;
    let stream = UnixListenerStream::new(listener);

    // Detect hardware
    let hardware_profile = hardware::HardwareDetector::detect();
    info!(
        "Hardware: {} RAM, {:?} GPU",
        hardware_profile.ram_bytes / (1024 * 1024 * 1024),
        hardware_profile.gpu_backend
    );

    // Select and load model
    let selector = model::ModelSelector::new();
    let model_spec = selector.select(&hardware_profile).context("Model selection failed")?;
    let model_name = model_spec.name.clone();
    info!("Selected model: {} (backend: {:?})", model_name, model_spec.backend);

    // Get models directory
    let models_dir = std::env::var("TSUKU_HOME")
        .ok()
        .map(PathBuf::from)
        .unwrap_or_else(|| {
            dirs::home_dir()
                .expect("Could not determine home directory")
                .join(".tsuku")
        })
        .join("models");

    // Ensure model is available - check for SIGTERM during download
    let model_manager = models::ModelManager::new(models_dir.clone());
    let model_path = model_manager.model_path(&model_name);

    if !model_manager.is_available(&model_name).await {
        info!("Model not found locally, downloading...");
        let download_future = model_manager.download(&model_name, |progress| {
            info!(
                "Download progress: {} bytes",
                progress.bytes_downloaded
            );
        });

        tokio::select! {
            result = download_future => {
                result.context("Failed to download model")?;
            }
            _ = sigterm.recv() => {
                info!("SIGTERM received during model download, cleaning up");
                cleanup_files(&socket, &lock);
                info!("Server shutdown complete (reason: SIGTERM during startup)");
                std::process::exit(0);
            }
        }
    }

    info!("Loading model from {:?}", model_path);

    // Load model (blocking operation, run in spawn_blocking)
    // Check for SIGTERM during model loading
    let model_params = match model_spec.backend {
        model::Backend::Cpu => ModelParams::for_cpu(),
        _ => ModelParams::for_gpu(),
    };
    let load_future = tokio::task::spawn_blocking({
        let path = model_path.clone();
        move || LlamaModel::load_from_file(&path, model_params)
    });

    let model = tokio::select! {
        result = load_future => {
            result
                .context("Model loading task panicked")?
                .context("Failed to load model")?
        }
        _ = sigterm.recv() => {
            info!("SIGTERM received during model loading, cleaning up");
            cleanup_files(&socket, &lock);
            info!("Server shutdown complete (reason: SIGTERM during startup)");
            std::process::exit(0);
        }
    };

    let model = Arc::new(model);
    info!("Model loaded successfully");

    // Create inference context
    let context_params = ContextParams::default();
    let context = LlamaContext::new(model.clone(), context_params).context("Failed to create context")?;
    info!("Inference context created");

    // Create shutdown channel
    let (shutdown_tx, mut shutdown_rx) = mpsc::channel::<()>(1);

    // Create the server
    let server = LlmServer::new(
        model_name,
        hardware_profile,
        shutdown_tx.clone(),
        model,
        context,
    );
    let shutting_down = server.shutting_down();
    let in_flight = server.in_flight();

    info!("Server ready, waiting for connections...");

    // Run the server with graceful shutdown
    let server_future = tonic::transport::Server::builder()
        .add_service(InferenceServiceServer::new(server))
        .serve_with_incoming_shutdown(stream, async {
            shutdown_rx.recv().await;
            info!("Shutdown signal received");
        });

    // Main event loop
    let shutdown_reason = tokio::select! {
        result = server_future => {
            result.context("Server error")?;
            "server stopped"
        }
        _ = tokio::time::sleep(idle_timeout) => {
            info!("Idle timeout reached, initiating shutdown");
            "idle timeout"
        }
        _ = sigterm.recv() => {
            info!("SIGTERM received, initiating graceful shutdown");
            "SIGTERM"
        }
    };

    // Mark server as shutting down
    shutting_down.store(true, Ordering::SeqCst);

    // Wait for in-flight requests with grace period
    // Pass sigterm so we can detect a second signal during grace period
    let _interrupted = wait_for_in_flight(&in_flight, SHUTDOWN_GRACE_PERIOD, &mut sigterm).await;

    // Clean up files
    cleanup_files(&socket, &lock);

    info!("Server shutdown complete (reason: {})", shutdown_reason);

    // Exit explicitly with code 0 to prevent the default signal handler
    // from terminating the process with "signal: terminated" status.
    // This ensures the process exits cleanly after all cleanup is done.
    std::process::exit(0);
}

// Bring in scopeguard for the in-flight request tracking RAII guard
mod scopeguard {
    pub fn guard<T, F: FnOnce(T)>(value: T, cleanup: F) -> Guard<T, F> {
        Guard {
            value: Some(value),
            cleanup: Some(cleanup),
        }
    }

    pub struct Guard<T, F: FnOnce(T)> {
        value: Option<T>,
        cleanup: Option<F>,
    }

    impl<T, F: FnOnce(T)> Drop for Guard<T, F> {
        fn drop(&mut self) {
            if let (Some(value), Some(cleanup)) = (self.value.take(), self.cleanup.take()) {
                cleanup(value);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_duration_seconds() {
        assert_eq!(parse_duration("30s").unwrap(), Duration::from_secs(30));
        assert_eq!(parse_duration("300s").unwrap(), Duration::from_secs(300));
    }

    #[test]
    fn test_parse_duration_minutes() {
        assert_eq!(parse_duration("5m").unwrap(), Duration::from_secs(300));
        assert_eq!(parse_duration("1m").unwrap(), Duration::from_secs(60));
    }

    #[test]
    fn test_parse_duration_hours() {
        assert_eq!(parse_duration("1h").unwrap(), Duration::from_secs(3600));
        assert_eq!(parse_duration("2h").unwrap(), Duration::from_secs(7200));
    }

    #[test]
    fn test_parse_duration_combined() {
        assert_eq!(parse_duration("1h30m").unwrap(), Duration::from_secs(5400));
        assert_eq!(parse_duration("2m30s").unwrap(), Duration::from_secs(150));
    }

    #[test]
    fn test_parse_duration_bare_number() {
        // Bare number treated as seconds
        assert_eq!(parse_duration("300").unwrap(), Duration::from_secs(300));
    }

    #[test]
    fn test_parse_duration_errors() {
        assert!(parse_duration("").is_err());
        assert!(parse_duration("0s").is_err()); // Zero duration not allowed
        assert!(parse_duration("abc").is_err());
    }
}
