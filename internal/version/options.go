package version

// Option configures a Resolver.
type Option func(*Resolver)

// WithNpmRegistry sets a custom npm registry URL for testing.
func WithNpmRegistry(url string) Option {
	return func(r *Resolver) {
		r.npmRegistryURL = url
	}
}

// WithPyPIRegistry sets a custom PyPI registry URL for testing.
func WithPyPIRegistry(url string) Option {
	return func(r *Resolver) {
		r.pypiRegistryURL = url
	}
}

// WithCratesIORegistry sets a custom crates.io registry URL for testing.
func WithCratesIORegistry(url string) Option {
	return func(r *Resolver) {
		r.cratesIORegistryURL = url
	}
}

// WithRubyGemsRegistry sets a custom RubyGems registry URL for testing.
func WithRubyGemsRegistry(url string) Option {
	return func(r *Resolver) {
		r.rubygemsRegistryURL = url
	}
}

// WithMetaCPANRegistry sets a custom MetaCPAN registry URL for testing.
func WithMetaCPANRegistry(url string) Option {
	return func(r *Resolver) {
		r.metacpanRegistryURL = url
	}
}

// WithHomebrewRegistry sets a custom Homebrew registry URL for testing.
func WithHomebrewRegistry(url string) Option {
	return func(r *Resolver) {
		r.homebrewRegistryURL = url
	}
}

// WithGoDevURL sets a custom go.dev URL for testing.
func WithGoDevURL(url string) Option {
	return func(r *Resolver) {
		r.goDevURL = url
	}
}

// WithGoProxyURL sets a custom Go proxy URL for testing.
func WithGoProxyURL(url string) Option {
	return func(r *Resolver) {
		r.goProxyURL = url
	}
}
