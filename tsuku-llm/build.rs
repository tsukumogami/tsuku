fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Compile the proto file
    tonic_build::configure()
        .build_server(true)
        .build_client(false) // Server only - Go has its own client
        .compile_protos(&["../proto/llm.proto"], &["../proto"])?;
    Ok(())
}
