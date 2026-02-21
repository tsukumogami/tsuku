//! Build script for tsuku-llm.
//!
//! This script:
//! 1. Compiles the gRPC proto file
//! 2. Compiles llama.cpp via cmake
//! 3. Generates Rust bindings via bindgen

use std::env;
use std::path::PathBuf;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    // 1. Compile the proto file
    tonic_build::configure()
        .build_server(true)
        .build_client(false) // Server only - Go has its own client
        .compile_protos(&["../proto/llm.proto"], &["../proto"])?;

    // 2. Compile llama.cpp via cmake
    compile_llama_cpp()?;

    // 3. Generate Rust bindings via bindgen
    generate_bindings()?;

    Ok(())
}

/// Compile llama.cpp static library via cmake.
fn compile_llama_cpp() -> Result<(), Box<dyn std::error::Error>> {
    let mut cmake_config = cmake::Config::new("llama.cpp");

    // Common settings
    cmake_config
        .define("BUILD_SHARED_LIBS", "OFF")
        .define("LLAMA_BUILD_TESTS", "OFF")
        .define("LLAMA_BUILD_EXAMPLES", "OFF")
        .define("LLAMA_BUILD_SERVER", "OFF");

    // Feature-gated GPU backends
    #[cfg(feature = "cuda")]
    {
        cmake_config.define("GGML_CUDA", "ON");
        // Allow overriding CUDA architectures via env var for cross-compilation
        // or when the toolkit is older than the installed GPU (e.g. toolkit 12.0
        // with Blackwell sm_120). Use "90-virtual" to generate PTX that the
        // driver can JIT-compile for newer architectures.
        if let Ok(archs) = env::var("TSUKU_CUDA_ARCHITECTURES") {
            cmake_config.define("CMAKE_CUDA_ARCHITECTURES", &archs);
            println!("cargo:warning=Building with CUDA support (architectures: {})", archs);
        } else {
            println!("cargo:warning=Building with CUDA support (native architectures)");
        }
    }

    #[cfg(not(feature = "cuda"))]
    {
        cmake_config.define("GGML_CUDA", "OFF");
    }

    #[cfg(feature = "metal")]
    {
        cmake_config.define("GGML_METAL", "ON");
        println!("cargo:warning=Building with Metal support");
    }

    #[cfg(not(feature = "metal"))]
    {
        cmake_config.define("GGML_METAL", "OFF");
    }

    #[cfg(feature = "vulkan")]
    {
        cmake_config.define("GGML_VULKAN", "ON");
        println!("cargo:warning=Building with Vulkan support");
    }

    #[cfg(not(feature = "vulkan"))]
    {
        cmake_config.define("GGML_VULKAN", "OFF");
    }

    // Build the library
    let dst = cmake_config.build();

    // Tell cargo where to find the libraries
    println!("cargo:rustc-link-search=native={}/lib", dst.display());
    println!("cargo:rustc-link-search=native={}/lib64", dst.display());

    // Link static libraries
    println!("cargo:rustc-link-lib=static=llama");
    println!("cargo:rustc-link-lib=static=ggml");
    println!("cargo:rustc-link-lib=static=ggml-base");
    println!("cargo:rustc-link-lib=static=ggml-cpu");

    // Link C++ standard library
    #[cfg(target_os = "linux")]
    {
        println!("cargo:rustc-link-lib=stdc++");
        // Link OpenMP for parallel CPU operations
        println!("cargo:rustc-link-lib=gomp");
    }

    #[cfg(target_os = "macos")]
    println!("cargo:rustc-link-lib=c++");

    // Link platform-specific libraries
    #[cfg(target_os = "macos")]
    {
        println!("cargo:rustc-link-lib=framework=Foundation");
        println!("cargo:rustc-link-lib=framework=Accelerate");

        #[cfg(feature = "metal")]
        {
            println!("cargo:rustc-link-lib=framework=Metal");
            println!("cargo:rustc-link-lib=framework=MetalKit");
            println!("cargo:rustc-link-lib=static=ggml-metal");
        }
    }

    #[cfg(feature = "cuda")]
    {
        println!("cargo:rustc-link-lib=cuda");
        println!("cargo:rustc-link-lib=cublas");
        println!("cargo:rustc-link-lib=culibos");
        println!("cargo:rustc-link-lib=cudart");
        println!("cargo:rustc-link-lib=cublasLt");
        println!("cargo:rustc-link-lib=static=ggml-cuda");
    }

    #[cfg(feature = "vulkan")]
    {
        println!("cargo:rustc-link-lib=vulkan");
        println!("cargo:rustc-link-lib=static=ggml-vulkan");
    }

    // Rerun if llama.cpp sources change
    println!("cargo:rerun-if-changed=llama.cpp/");

    Ok(())
}

/// Generate Rust bindings for llama.cpp via bindgen.
fn generate_bindings() -> Result<(), Box<dyn std::error::Error>> {
    let bindings = bindgen::Builder::default()
        // Input header
        .header("llama.cpp/include/llama.h")
        // Include path
        .clang_arg("-Illama.cpp/include")
        .clang_arg("-Illama.cpp/ggml/include")
        // Add system include paths for GCC headers
        .clang_arg("-I/usr/lib/gcc/x86_64-linux-gnu/13/include")
        .clang_arg("-I/usr/include")
        // Use C mode (llama.h is a C header)
        .clang_arg("-x")
        .clang_arg("c")
        // Generate bindings for these functions
        .allowlist_function("llama_.*")
        .allowlist_function("ggml_.*")
        // Generate bindings for these types
        .allowlist_type("llama_.*")
        .allowlist_type("ggml_.*")
        // Generate bindings for these variables
        .allowlist_var("LLAMA_.*")
        .allowlist_var("GGML_.*")
        // Derive common traits
        .derive_debug(true)
        .derive_default(true)
        .derive_copy(true)
        // Don't generate layout tests (they fail across different environments)
        .layout_tests(false)
        // Use core instead of std where possible
        .use_core()
        // Generate the bindings
        .generate()
        .map_err(|e| format!("Failed to generate bindings: {}", e))?;

    // Write bindings to OUT_DIR
    let out_path = PathBuf::from(env::var("OUT_DIR")?);
    bindings.write_to_file(out_path.join("bindings.rs"))?;

    // Rerun if header changes
    println!("cargo:rerun-if-changed=llama.cpp/include/llama.h");
    println!("cargo:rerun-if-changed=llama.cpp/ggml/include/ggml.h");

    Ok(())
}
