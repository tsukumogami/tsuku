//! tsuku-llm: Local LLM inference server for tsuku.
//!
//! This binary provides local inference capabilities via gRPC over Unix domain sockets.
//! It bundles llama.cpp and handles hardware detection, model management, and inference.

mod hardware;

use std::fs::File;
use std::os::unix::io::AsRawFd;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{bail, Context, Result};
use clap::{Parser, Subcommand};
use tokio::net::UnixListener;
use tokio::sync::mpsc;
use tokio_stream::wrappers::UnixListenerStream;
use tonic::{Request, Response, Status};
use tracing::{info, warn};

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
}

impl LlmServer {
    fn new(model_name: String, hardware_profile: hardware::HardwareProfile, shutdown_tx: mpsc::Sender<()>) -> Self {
        Self {
            model_name,
            hardware_profile,
            shutdown_tx,
            shutting_down: Arc::new(AtomicBool::new(false)),
            in_flight: Arc::new(AtomicUsize::new(0)),
        }
    }

    fn shutting_down(&self) -> Arc<AtomicBool> {
        self.shutting_down.clone()
    }

    fn in_flight(&self) -> Arc<AtomicUsize> {
        self.in_flight.clone()
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

        // TODO: Actual inference via llama.cpp
        // For now, return a stub response
        let response = CompletionResponse {
            content: format!(
                "[STUB] Model {} received prompt with {} messages",
                self.model_name,
                req.messages.len()
            ),
            tool_calls: vec![],
            stop_reason: "end_turn".to_string(),
            usage: Some(Usage {
                input_tokens: 100, // Placeholder
                output_tokens: 50, // Placeholder
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
async fn wait_for_in_flight(in_flight: &Arc<AtomicUsize>, timeout: Duration) {
    let start = std::time::Instant::now();

    loop {
        let count = in_flight.load(Ordering::SeqCst);
        if count == 0 {
            info!("All in-flight requests completed");
            return;
        }

        if start.elapsed() >= timeout {
            warn!(
                "Grace period expired with {} in-flight requests",
                count
            );
            return;
        }

        info!(
            "Waiting for {} in-flight requests ({:.1}s remaining)",
            count,
            (timeout - start.elapsed()).as_secs_f32()
        );
        tokio::time::sleep(Duration::from_millis(100)).await;
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

    // Create shutdown channel
    let (shutdown_tx, mut shutdown_rx) = mpsc::channel::<()>(1);

    // Create the server
    let model_name = "stub-model".to_string(); // TODO: Load actual model based on hardware
    let server = LlmServer::new(model_name, hardware_profile, shutdown_tx.clone());
    let shutting_down = server.shutting_down();
    let in_flight = server.in_flight();

    info!("Server ready, waiting for connections...");

    // Set up SIGTERM handler
    #[cfg(unix)]
    let mut sigterm = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
        .context("Failed to register SIGTERM handler")?;

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
    wait_for_in_flight(&in_flight, SHUTDOWN_GRACE_PERIOD).await;

    // Clean up files
    cleanup_files(&socket, &lock);

    info!("Server shutdown complete (reason: {})", shutdown_reason);
    Ok(())
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
