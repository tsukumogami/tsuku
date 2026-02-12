//! tsuku-llm: Local LLM inference server for tsuku.
//!
//! This binary provides local inference capabilities via gRPC over Unix domain sockets.
//! It bundles llama.cpp and handles hardware detection, model management, and inference.

use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
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

/// Inference server implementation.
struct LlmServer {
    /// Loaded model name.
    model_name: String,

    /// Signal to initiate shutdown.
    shutdown_tx: mpsc::Sender<()>,

    /// Whether the server is shutting down.
    shutting_down: Arc<AtomicBool>,
}

impl LlmServer {
    fn new(model_name: String, shutdown_tx: mpsc::Sender<()>) -> Self {
        Self {
            model_name,
            shutdown_tx,
            shutting_down: Arc::new(AtomicBool::new(false)),
        }
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
        // TODO: Real hardware detection and model info
        let response = StatusResponse {
            ready: !self.shutting_down.load(Ordering::SeqCst),
            model_name: self.model_name.clone(),
            model_size_bytes: 0, // TODO: Actual model size
            backend: "cpu".to_string(), // TODO: Detect hardware
            available_vram_bytes: 0,
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

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize logging
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive("tsuku_llm=info".parse().unwrap()),
        )
        .init();

    let socket_path = socket_path();
    info!("Starting tsuku-llm server at {:?}", socket_path);

    // Remove stale socket if it exists
    if socket_path.exists() {
        std::fs::remove_file(&socket_path).context("Failed to remove stale socket")?;
    }

    // Ensure parent directory exists
    if let Some(parent) = socket_path.parent() {
        std::fs::create_dir_all(parent).context("Failed to create socket directory")?;
    }

    // Create Unix listener
    let listener = UnixListener::bind(&socket_path).context("Failed to bind Unix socket")?;
    let stream = UnixListenerStream::new(listener);

    // Create shutdown channel
    let (shutdown_tx, mut shutdown_rx) = mpsc::channel::<()>(1);

    // Create the server
    let model_name = "stub-model".to_string(); // TODO: Load actual model
    let server = LlmServer::new(model_name, shutdown_tx);

    info!("Server ready, waiting for connections...");

    // Run the server with graceful shutdown
    let server_future = tonic::transport::Server::builder()
        .add_service(InferenceServiceServer::new(server))
        .serve_with_incoming_shutdown(stream, async {
            shutdown_rx.recv().await;
            info!("Shutdown signal received");
        });

    // Set up idle timeout (5 minutes)
    let idle_timeout = Duration::from_secs(5 * 60);
    tokio::select! {
        result = server_future => {
            result.context("Server error")?;
        }
        _ = tokio::time::sleep(idle_timeout) => {
            info!("Idle timeout reached, shutting down");
        }
    }

    // Clean up socket
    if socket_path.exists() {
        std::fs::remove_file(&socket_path).ok();
    }

    info!("Server shutdown complete");
    Ok(())
}
