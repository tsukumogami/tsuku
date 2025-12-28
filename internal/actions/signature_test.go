package actions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

func TestValidateFingerprint(t *testing.T) {
	tests := []struct {
		name        string
		fingerprint string
		wantErr     bool
	}{
		{
			name:        "valid lowercase fingerprint",
			fingerprint: "d53626f8174a9846f6a573cc1253fa47ea19e301",
			wantErr:     false,
		},
		{
			name:        "valid uppercase fingerprint",
			fingerprint: "D53626F8174A9846F6A573CC1253FA47EA19E301",
			wantErr:     false,
		},
		{
			name:        "valid mixed case fingerprint",
			fingerprint: "D53626f8174A9846f6a573CC1253FA47EA19E301",
			wantErr:     false,
		},
		{
			name:        "too short",
			fingerprint: "D53626F8174A9846F6A573CC1253FA47EA19E3",
			wantErr:     true,
		},
		{
			name:        "too long",
			fingerprint: "D53626F8174A9846F6A573CC1253FA47EA19E30100",
			wantErr:     true,
		},
		{
			name:        "empty string",
			fingerprint: "",
			wantErr:     true,
		},
		{
			name:        "contains invalid characters",
			fingerprint: "D53626F8174A9846F6A573CC1253FA47EA19GHIJ",
			wantErr:     true,
		},
		{
			name:        "contains spaces",
			fingerprint: "D536 26F8 174A 9846 F6A5 73CC 1253 FA47 EA19 E301",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFingerprint(tt.fingerprint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFingerprint() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeFingerprint(t *testing.T) {
	tests := []struct {
		name        string
		fingerprint string
		want        string
	}{
		{
			name:        "lowercase to uppercase",
			fingerprint: "d53626f8174a9846f6a573cc1253fa47ea19e301",
			want:        "D53626F8174A9846F6A573CC1253FA47EA19E301",
		},
		{
			name:        "already uppercase",
			fingerprint: "D53626F8174A9846F6A573CC1253FA47EA19E301",
			want:        "D53626F8174A9846F6A573CC1253FA47EA19E301",
		},
		{
			name:        "mixed case",
			fingerprint: "D53626f8174A9846f6a573CC1253FA47ea19e301",
			want:        "D53626F8174A9846F6A573CC1253FA47EA19E301",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeFingerprint(tt.fingerprint)
			if got != tt.want {
				t.Errorf("NormalizeFingerprint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatFingerprint(t *testing.T) {
	tests := []struct {
		name string
		fp   string
		want string
	}{
		{
			name: "40 char fingerprint",
			fp:   "D53626F8174A9846F6A573CC1253FA47EA19E301",
			want: "D536 26F8 174A 9846 F6A5 73CC 1253 FA47 EA19 E301",
		},
		{
			name: "lowercase gets uppercased",
			fp:   "d53626f8174a9846f6a573cc1253fa47ea19e301",
			want: "D536 26F8 174A 9846 F6A5 73CC 1253 FA47 EA19 E301",
		},
		{
			name: "already has spaces - removes them first",
			fp:   "D536 26F8 174A 9846 F6A5 73CC 1253 FA47 EA19 E301",
			want: "D536 26F8 174A 9846 F6A5 73CC 1253 FA47 EA19 E301",
		},
		{
			name: "short fingerprint returned as-is",
			fp:   "ABCD1234",
			want: "ABCD1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatFingerprint(tt.fp)
			if got != tt.want {
				t.Errorf("FormatFingerprint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseFingerprint(t *testing.T) {
	tests := []struct {
		name    string
		fp      string
		want    string
		wantErr bool
	}{
		{
			name:    "clean fingerprint",
			fp:      "d53626f8174a9846f6a573cc1253fa47ea19e301",
			want:    "D53626F8174A9846F6A573CC1253FA47EA19E301",
			wantErr: false,
		},
		{
			name:    "fingerprint with spaces",
			fp:      "D536 26F8 174A 9846 F6A5 73CC 1253 FA47 EA19 E301",
			want:    "D53626F8174A9846F6A573CC1253FA47EA19E301",
			wantErr: false,
		},
		{
			name:    "too short",
			fp:      "D53626F8174A",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid hex",
			fp:      "ZZZZ26F8174A9846F6A573CC1253FA47EA19E301",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFingerprint(tt.fp)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFingerprint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseFingerprint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPGPKeyCache_SaveAndLoad(t *testing.T) {
	// Create a temporary cache directory
	cacheDir := t.TempDir()
	cache := NewPGPKeyCache(cacheDir)

	// Generate a test key pair
	key, err := crypto.GenerateKey("Test", "test@example.com", "rsa", 2048)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	fingerprint := NormalizeFingerprint(key.GetFingerprint())
	armoredKey, err := key.Armor()
	if err != nil {
		t.Fatalf("Failed to armor key: %v", err)
	}

	// Save to cache
	err = cache.saveToCache(fingerprint, armoredKey)
	if err != nil {
		t.Fatalf("saveToCache() error = %v", err)
	}

	// Verify file was created
	cachePath := filepath.Join(cacheDir, fingerprint+".asc")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Error("Cache file was not created")
	}

	// Load from cache
	loadedKey, err := cache.loadFromCache(fingerprint)
	if err != nil {
		t.Fatalf("loadFromCache() error = %v", err)
	}

	// Verify fingerprint matches
	loadedFingerprint := NormalizeFingerprint(loadedKey.GetFingerprint())
	if loadedFingerprint != fingerprint {
		t.Errorf("Loaded key fingerprint = %v, want %v", loadedFingerprint, fingerprint)
	}
}

func TestPGPKeyCache_FingerprintMismatch(t *testing.T) {
	cacheDir := t.TempDir()
	cache := NewPGPKeyCache(cacheDir)

	// Generate a test key
	key, err := crypto.GenerateKey("Test", "test@example.com", "rsa", 2048)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	armoredKey, err := key.Armor()
	if err != nil {
		t.Fatalf("Failed to armor key: %v", err)
	}

	// Save with wrong fingerprint
	wrongFingerprint := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	err = cache.saveToCache(wrongFingerprint, armoredKey)
	if err != nil {
		t.Fatalf("saveToCache() error = %v", err)
	}

	// Try to load - should fail due to fingerprint mismatch
	_, err = cache.loadFromCache(wrongFingerprint)
	if err == nil {
		t.Error("loadFromCache() should fail with fingerprint mismatch")
	}

	// Verify cache file was removed
	cachePath := filepath.Join(cacheDir, wrongFingerprint+".asc")
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Error("Cache file should have been removed after fingerprint mismatch")
	}
}

func TestPGPKeyCache_FetchKey(t *testing.T) {
	// Generate a test key
	key, err := crypto.GenerateKey("Test", "test@example.com", "rsa", 2048)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	fingerprint := NormalizeFingerprint(key.GetFingerprint())
	armoredKey, err := key.Armor()
	if err != nil {
		t.Fatalf("Failed to armor key: %v", err)
	}

	// Set up test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(armoredKey))
	}))
	defer server.Close()

	cache := NewPGPKeyCache(t.TempDir())

	// Fetch key
	fetchedKey, armoredResult, err := cache.fetchKey(context.Background(), server.URL, fingerprint)
	if err != nil {
		t.Fatalf("fetchKey() error = %v", err)
	}

	// Verify fingerprint matches
	fetchedFingerprint := NormalizeFingerprint(fetchedKey.GetFingerprint())
	if fetchedFingerprint != fingerprint {
		t.Errorf("Fetched key fingerprint = %v, want %v", fetchedFingerprint, fingerprint)
	}

	// Verify armored key was returned
	if armoredResult != armoredKey {
		t.Error("Armored key should be returned unchanged")
	}
}

func TestPGPKeyCache_FetchKeyWrongFingerprint(t *testing.T) {
	// Generate a test key
	key, err := crypto.GenerateKey("Test", "test@example.com", "rsa", 2048)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	armoredKey, err := key.Armor()
	if err != nil {
		t.Fatalf("Failed to armor key: %v", err)
	}

	// Set up test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(armoredKey))
	}))
	defer server.Close()

	cache := NewPGPKeyCache(t.TempDir())

	// Try to fetch with wrong expected fingerprint
	wrongFingerprint := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	_, _, err = cache.fetchKey(context.Background(), server.URL, wrongFingerprint)
	if err == nil {
		t.Error("fetchKey() should fail with fingerprint mismatch")
	}
}

func TestPGPKeyCache_Get(t *testing.T) {
	// Generate a test key
	key, err := crypto.GenerateKey("Test", "test@example.com", "rsa", 2048)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	fingerprint := NormalizeFingerprint(key.GetFingerprint())
	armoredKey, err := key.Armor()
	if err != nil {
		t.Fatalf("Failed to armor key: %v", err)
	}

	// Set up test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(armoredKey))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	cache := NewPGPKeyCache(cacheDir)

	// First call - should fetch from server
	fetchedKey, err := cache.Get(context.Background(), fingerprint, server.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	fetchedFingerprint := NormalizeFingerprint(fetchedKey.GetFingerprint())
	if fetchedFingerprint != fingerprint {
		t.Errorf("Fetched key fingerprint = %v, want %v", fetchedFingerprint, fingerprint)
	}

	// Verify key was cached
	cachePath := filepath.Join(cacheDir, fingerprint+".asc")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Error("Key should have been cached")
	}

	// Second call with bad URL - should load from cache
	cachedKey, err := cache.Get(context.Background(), fingerprint, "http://invalid-url-that-should-not-be-called")
	if err != nil {
		t.Fatalf("Get() with cache error = %v", err)
	}

	cachedFingerprint := NormalizeFingerprint(cachedKey.GetFingerprint())
	if cachedFingerprint != fingerprint {
		t.Errorf("Cached key fingerprint = %v, want %v", cachedFingerprint, fingerprint)
	}
}

func TestVerifyPGPSignature(t *testing.T) {
	// Generate a test key pair (GenerateKey returns a key with both private and public parts)
	key, err := crypto.GenerateKey("Test", "test@example.com", "rsa", 2048)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	// Test data
	testData := []byte("Hello, this is test data for signature verification!")

	// Create a temporary file with the test data
	tmpFile := filepath.Join(t.TempDir(), "testfile")
	if err := os.WriteFile(tmpFile, testData, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create a keyring from the private key for signing
	signingKeyRing, err := crypto.NewKeyRing(key)
	if err != nil {
		t.Fatalf("Failed to create signing keyring: %v", err)
	}

	// Sign the data
	message := crypto.NewPlainMessage(testData)
	signature, err := signingKeyRing.SignDetached(message)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	armoredSig, err := signature.GetArmored()
	if err != nil {
		t.Fatalf("Failed to armor signature: %v", err)
	}

	// Get public key for verification
	publicKey, err := key.ToPublic()
	if err != nil {
		t.Fatalf("Failed to get public key: %v", err)
	}

	// Test successful verification
	t.Run("valid signature", func(t *testing.T) {
		err := VerifyPGPSignature(context.Background(), tmpFile, []byte(armoredSig), publicKey)
		if err != nil {
			t.Errorf("VerifyPGPSignature() error = %v, want nil", err)
		}
	})

	// Test verification with wrong data
	t.Run("wrong data", func(t *testing.T) {
		wrongFile := filepath.Join(t.TempDir(), "wrongfile")
		if err := os.WriteFile(wrongFile, []byte("wrong data"), 0644); err != nil {
			t.Fatalf("Failed to write wrong file: %v", err)
		}

		err := VerifyPGPSignature(context.Background(), wrongFile, []byte(armoredSig), publicKey)
		if err == nil {
			t.Error("VerifyPGPSignature() should fail with wrong data")
		}
	})

	// Test verification with wrong key
	t.Run("wrong key", func(t *testing.T) {
		wrongKey, err := crypto.GenerateKey("Wrong", "wrong@example.com", "rsa", 2048)
		if err != nil {
			t.Fatalf("Failed to generate wrong key: %v", err)
		}

		wrongPublicKey, err := wrongKey.ToPublic()
		if err != nil {
			t.Fatalf("Failed to get wrong public key: %v", err)
		}

		err = VerifyPGPSignature(context.Background(), tmpFile, []byte(armoredSig), wrongPublicKey)
		if err == nil {
			t.Error("VerifyPGPSignature() should fail with wrong key")
		}
	})

	// Test with binary signature
	t.Run("binary signature", func(t *testing.T) {
		binarySig := signature.GetBinary()
		err := VerifyPGPSignature(context.Background(), tmpFile, binarySig, publicKey)
		if err != nil {
			t.Errorf("VerifyPGPSignature() with binary signature error = %v, want nil", err)
		}
	})
}

func TestFetchSignature(t *testing.T) {
	testSig := []byte("-----BEGIN PGP SIGNATURE-----\n\ntest signature data\n-----END PGP SIGNATURE-----")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testSig)
	}))
	defer server.Close()

	t.Run("successful fetch", func(t *testing.T) {
		sig, err := FetchSignature(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("FetchSignature() error = %v", err)
		}

		if string(sig) != string(testSig) {
			t.Errorf("FetchSignature() = %v, want %v", string(sig), string(testSig))
		}
	})

	t.Run("HTTP error", func(t *testing.T) {
		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer errorServer.Close()

		_, err := FetchSignature(context.Background(), errorServer.URL)
		if err == nil {
			t.Error("FetchSignature() should fail with HTTP error")
		}
	})
}

func TestGetKeyFingerprint(t *testing.T) {
	key, err := crypto.GenerateKey("Test", "test@example.com", "rsa", 2048)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	result := GetKeyFingerprint(key)

	// Should be formatted with spaces
	if len(result) != 49 { // 40 chars + 9 spaces
		t.Errorf("GetKeyFingerprint() result length = %d, want 49", len(result))
	}

	// Should contain spaces at expected positions
	parts := make([]string, 10)
	for i := 0; i < 10; i++ {
		start := i * 5 // Each part is 4 chars + 1 space (except last)
		if i < 9 {
			parts[i] = result[start : start+4]
		} else {
			parts[i] = result[start : start+4]
		}
	}

	// Verify all parts are 4 chars and uppercase hex
	for i, part := range parts {
		if len(part) != 4 {
			t.Errorf("Part %d length = %d, want 4", i, len(part))
		}
		for _, c := range part {
			if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')) {
				t.Errorf("Part %d contains non-hex character: %c", i, c)
			}
		}
	}
}
