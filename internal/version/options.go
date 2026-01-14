package version

// Option is a functional option for configuring the Resolver
type Option func(*Resolver)

// WithNpmRegistry sets a custom npm registry URL
func WithNpmRegistry(url string) Option {
	return func(r *Resolver) {
		r.npmRegistryURL = url
	}
}

// WithPyPIRegistry sets a custom PyPI registry URL
func WithPyPIRegistry(url string) Option {
	return func(r *Resolver) {
		r.pypiRegistryURL = url
	}
}

// WithCratesIORegistry sets a custom crates.io registry URL
func WithCratesIORegistry(url string) Option {
	return func(r *Resolver) {
		r.cratesIORegistryURL = url
	}
}

// WithRubyGemsRegistry sets a custom RubyGems registry URL
func WithRubyGemsRegistry(url string) Option {
	return func(r *Resolver) {
		r.rubygemsRegistryURL = url
	}
}

// WithMetaCPANRegistry sets a custom MetaCPAN registry URL
func WithMetaCPANRegistry(url string) Option {
	return func(r *Resolver) {
		r.metacpanRegistryURL = url
	}
}

// WithHomebrewRegistry sets a custom Homebrew registry URL
func WithHomebrewRegistry(url string) Option {
	return func(r *Resolver) {
		r.homebrewRegistryURL = url
	}
}

// WithCaskRegistry sets a custom Homebrew Cask registry URL
func WithCaskRegistry(url string) Option {
	return func(r *Resolver) {
		r.caskRegistryURL = url
	}
}

// WithGoDevURL sets a custom go.dev URL
func WithGoDevURL(url string) Option {
	return func(r *Resolver) {
		r.goDevURL = url
	}
}

// WithGoProxyURL sets a custom Go proxy URL
func WithGoProxyURL(url string) Option {
	return func(r *Resolver) {
		r.goProxyURL = url
	}
}
