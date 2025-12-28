package actions

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/tsukumogami/tsuku/internal/httputil"
)

const (
	// MaxKeySize is the maximum allowed size for a PGP public key (100KB).
	MaxKeySize = 100 * 1024

	// KeyFetchTimeout is the timeout for fetching a key from a URL.
	KeyFetchTimeout = 30 * time.Second
)

// fingerprintRegex matches valid 40-character hex fingerprints.
var fingerprintRegex = regexp.MustCompile(`^[0-9A-Fa-f]{40}$`)

// ValidateFingerprint checks if a fingerprint is valid (40 hex characters).
func ValidateFingerprint(fingerprint string) error {
	if !fingerprintRegex.MatchString(fingerprint) {
		return fmt.Errorf("invalid fingerprint format: must be 40 hex characters, got %q", fingerprint)
	}
	return nil
}

// NormalizeFingerprint converts a fingerprint to uppercase for consistent comparison.
func NormalizeFingerprint(fingerprint string) string {
	return strings.ToUpper(fingerprint)
}

// PGPKeyCache manages cached PGP public keys.
type PGPKeyCache struct {
	cacheDir string
}

// NewPGPKeyCache creates a new key cache in the specified directory.
func NewPGPKeyCache(cacheDir string) *PGPKeyCache {
	return &PGPKeyCache{cacheDir: cacheDir}
}

// Get retrieves a key by fingerprint, fetching from URL if not cached.
// The key is validated against the expected fingerprint before being returned.
func (c *PGPKeyCache) Get(ctx context.Context, fingerprint, keyURL string) (*crypto.Key, error) {
	fingerprint = NormalizeFingerprint(fingerprint)

	// Try to load from cache first
	key, err := c.loadFromCache(fingerprint)
	if err == nil {
		return key, nil
	}

	// Fetch from URL
	key, armoredKey, err := c.fetchKey(ctx, keyURL, fingerprint)
	if err != nil {
		return nil, err
	}

	// Save to cache
	if err := c.saveToCache(fingerprint, armoredKey); err != nil {
		// Log warning but don't fail - key is still usable
		fmt.Printf("   Warning: failed to cache key: %v\n", err)
	}

	return key, nil
}

// loadFromCache attempts to load a key from the cache directory.
func (c *PGPKeyCache) loadFromCache(fingerprint string) (*crypto.Key, error) {
	cachePath := filepath.Join(c.cacheDir, fingerprint+".asc")

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	key, err := crypto.NewKeyFromArmored(string(data))
	if err != nil {
		// Cached key is corrupted, remove it
		os.Remove(cachePath)
		return nil, fmt.Errorf("cached key is invalid: %w", err)
	}

	// Validate fingerprint matches
	keyFingerprint := strings.ToUpper(key.GetFingerprint())
	if keyFingerprint != fingerprint {
		// Cache file has wrong key - remove it
		os.Remove(cachePath)
		return nil, fmt.Errorf("cached key fingerprint mismatch")
	}

	return key, nil
}

// fetchKey downloads a key from a URL and validates its fingerprint.
func (c *PGPKeyCache) fetchKey(ctx context.Context, keyURL, expectedFingerprint string) (*crypto.Key, string, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, KeyFetchTimeout)
	defer cancel()

	// Create HTTP client with security protections
	client := httputil.NewSecureClient(httputil.ClientOptions{
		Timeout: KeyFetchTimeout,
	})

	req, err := http.NewRequestWithContext(ctx, "GET", keyURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create key request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch key from %s: %w", keyURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to fetch key: HTTP %d", resp.StatusCode)
	}

	// Limit response size to prevent resource exhaustion
	limitedReader := io.LimitReader(resp.Body, MaxKeySize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read key: %w", err)
	}

	if len(data) > MaxKeySize {
		return nil, "", fmt.Errorf("key exceeds maximum size of %d bytes", MaxKeySize)
	}

	armoredKey := string(data)
	key, err := crypto.NewKeyFromArmored(armoredKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse PGP key: %w", err)
	}

	// Validate fingerprint
	keyFingerprint := strings.ToUpper(key.GetFingerprint())
	if keyFingerprint != expectedFingerprint {
		return nil, "", fmt.Errorf("key fingerprint mismatch: expected %s, got %s", expectedFingerprint, keyFingerprint)
	}

	return key, armoredKey, nil
}

// saveToCache stores an armored key in the cache directory.
func (c *PGPKeyCache) saveToCache(fingerprint, armoredKey string) error {
	// Ensure cache directory exists with secure permissions
	if err := os.MkdirAll(c.cacheDir, 0700); err != nil {
		return err
	}

	cachePath := filepath.Join(c.cacheDir, fingerprint+".asc")

	// Write with restricted permissions
	return os.WriteFile(cachePath, []byte(armoredKey), 0600)
}

// VerifyPGPSignature verifies a file's detached PGP signature.
// It fetches the signature from signatureURL and the key from keyURL,
// validates the key fingerprint, and verifies the signature.
func VerifyPGPSignature(
	ctx context.Context,
	filePath string,
	signatureData []byte,
	key *crypto.Key,
) error {
	// Read the file to verify
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file for signature verification: %w", err)
	}

	// Parse the signature
	signature, err := crypto.NewPGPSignatureFromArmored(string(signatureData))
	if err != nil {
		// Try as binary signature
		signature = crypto.NewPGPSignature(signatureData)
	}

	// Create a keyring with the public key
	keyRing, err := crypto.NewKeyRing(key)
	if err != nil {
		return fmt.Errorf("failed to create keyring: %w", err)
	}

	// Create a message from the file data
	message := crypto.NewPlainMessage(fileData)

	// Verify the signature
	// Use 0 for verifyTime to accept signatures at any time
	if err := keyRing.VerifyDetached(message, signature, 0); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	return nil
}

// FetchSignature downloads a signature file from a URL.
func FetchSignature(ctx context.Context, signatureURL string) ([]byte, error) {
	client := httputil.NewSecureClient(httputil.ClientOptions{
		Timeout: KeyFetchTimeout,
	})

	req, err := http.NewRequestWithContext(ctx, "GET", signatureURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create signature request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch signature from %s: %w", signatureURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch signature: HTTP %d", resp.StatusCode)
	}

	// Signatures are small, limit to 10KB
	limitedReader := io.LimitReader(resp.Body, 10*1024+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read signature: %w", err)
	}

	if len(data) > 10*1024 {
		return nil, fmt.Errorf("signature exceeds maximum size of 10KB")
	}

	return data, nil
}

// GetKeyFingerprint extracts and normalizes the fingerprint from a PGP key.
// This is useful for debugging and display purposes.
func GetKeyFingerprint(key *crypto.Key) string {
	fp := key.GetFingerprint()
	// Format as groups of 4 for readability
	return FormatFingerprint(fp)
}

// FormatFingerprint formats a fingerprint in the standard GPG format (groups of 4).
func FormatFingerprint(fp string) string {
	fp = strings.ToUpper(strings.ReplaceAll(fp, " ", ""))
	if len(fp) != 40 {
		return fp
	}

	var parts []string
	for i := 0; i < 40; i += 4 {
		parts = append(parts, fp[i:i+4])
	}
	return strings.Join(parts, " ")
}

// ParseFingerprint normalizes a fingerprint by removing spaces and converting to uppercase.
// Returns error if the result is not a valid 40-character hex string.
func ParseFingerprint(fp string) (string, error) {
	fp = strings.ToUpper(strings.ReplaceAll(fp, " ", ""))
	if len(fp) != 40 {
		return "", fmt.Errorf("fingerprint must be 40 hex characters, got %d", len(fp))
	}

	// Validate hex
	if _, err := hex.DecodeString(fp); err != nil {
		return "", fmt.Errorf("fingerprint contains invalid hex characters: %w", err)
	}

	return fp, nil
}
