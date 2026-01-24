use libc::{dlclose, dlerror, dlopen, RTLD_LOCAL, RTLD_NOW};
use serde::Serialize;
use std::env;
use std::ffi::{CStr, CString};
use std::io::{self, Write};
use std::process::ExitCode;

const VERSION: &str = env!("CARGO_PKG_VERSION");

#[derive(Serialize)]
struct Output {
    path: String,
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

fn main() -> ExitCode {
    let args: Vec<String> = env::args().skip(1).collect();

    if args.is_empty() {
        eprintln!("usage: tsuku-dltest <path>...");
        return ExitCode::from(2);
    }

    if args.len() == 1 && args[0] == "--version" {
        eprintln!("tsuku-dltest v{}", VERSION);
        return ExitCode::from(0);
    }

    let mut all_ok = true;
    let mut results = Vec::with_capacity(args.len());

    for path in &args {
        let result = match try_load(path) {
            Ok(()) => Output {
                path: path.clone(),
                ok: true,
                error: None,
            },
            Err(e) => {
                all_ok = false;
                Output {
                    path: path.clone(),
                    ok: false,
                    error: Some(e),
                }
            }
        };
        results.push(result);
    }

    serde_json::to_writer(io::stdout(), &results).unwrap();
    io::stdout().flush().unwrap();

    if all_ok {
        ExitCode::from(0)
    } else {
        ExitCode::from(1)
    }
}

/// Attempt to load a library with dlopen, then immediately unload it.
fn try_load(path: &str) -> Result<(), String> {
    let c_path = CString::new(path).map_err(|_| "path contains null byte".to_string())?;

    unsafe {
        // Clear any previous error (critical for correct error reporting)
        dlerror();

        let handle = dlopen(c_path.as_ptr(), RTLD_NOW | RTLD_LOCAL);

        if handle.is_null() {
            let err = dlerror();
            if err.is_null() {
                return Err("dlopen failed with unknown error".to_string());
            }
            return Err(CStr::from_ptr(err).to_string_lossy().into_owned());
        }

        // Check dlclose return value - failure is worth reporting
        if dlclose(handle) != 0 {
            let err = dlerror();
            if !err.is_null() {
                return Err(format!(
                    "dlclose failed: {}",
                    CStr::from_ptr(err).to_string_lossy()
                ));
            }
        }
    }

    Ok(())
}
