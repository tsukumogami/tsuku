use std::env;

const VERSION: &str = env!("CARGO_PKG_VERSION");

fn main() {
    let args: Vec<String> = env::args().collect();

    if args.len() == 2 && args[1] == "--version" {
        eprintln!("tsuku-dltest v{}", VERSION);
        std::process::exit(0);
    }

    eprintln!("tsuku-dltest v{}", VERSION);
    eprintln!("dlopen functionality not yet implemented");
    eprintln!("usage: tsuku-dltest <path>...");
    std::process::exit(2);
}
