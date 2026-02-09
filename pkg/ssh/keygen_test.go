package ssh

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/unifabric-io/nvair-cli/pkg/constant"
)

// TestGenerateKeyPair tests key pair generation.
func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Verify that key pair has content
	if len(kp.PrivateKey) == 0 {
		t.Error("Generated private key is empty")
	}
	if len(kp.PublicKey) == 0 {
		t.Error("Generated public key is empty")
	}
	if kp.Fingerprint == "" {
		t.Error("Generated fingerprint is empty")
	}

	// Verify fingerprint is valid base64
	_, err = base64.StdEncoding.DecodeString(kp.Fingerprint)
	if err != nil {
		t.Errorf("Fingerprint is not valid base64: %v", err)
	}
}

// TestSaveAndLoadKeyPair tests saving and loading a key pair.
func TestSaveAndLoadKeyPair(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	keyPath := filepath.Join(tmpDir, ".ssh", constant.DefaultKeyName)

	// Generate a key pair
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Save the key pair
	if err := kp.SaveKeyPair(keyPath); err != nil {
		t.Fatalf("Failed to save key pair: %v", err)
	}

	// Load the key pair back
	loaded, err := LoadKeyPair(keyPath)
	if err != nil {
		t.Fatalf("Failed to load key pair: %v", err)
	}

	// Verify content matches
	if string(loaded.PrivateKey) != string(kp.PrivateKey) {
		t.Error("Loaded private key doesn't match saved key")
	}
	if string(loaded.PublicKey) != string(kp.PublicKey) {
		t.Error("Loaded public key doesn't match saved key")
	}
	if loaded.Fingerprint != kp.Fingerprint {
		t.Error("Loaded fingerprint doesn't match saved fingerprint")
	}
}

// TestSaveKeyPairPermissions tests that keys are saved with correct permissions.
func TestSaveKeyPairPermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	keyPath := filepath.Join(tmpDir, ".ssh", constant.DefaultKeyName)

	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	if err := kp.SaveKeyPair(keyPath); err != nil {
		t.Fatalf("Failed to save key pair: %v", err)
	}

	// Check private key permissions (should be 0600)
	privInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("Failed to stat private key: %v", err)
	}

	privMode := privInfo.Mode() & os.ModePerm
	expectedPrivMode := os.FileMode(0600)
	if privMode != expectedPrivMode {
		t.Errorf("Private key permissions mismatch: got %#o, want %#o", privMode, expectedPrivMode)
	}

	// Check public key permissions (should be 0644)
	pubPath := keyPath + ".pub"
	pubInfo, err := os.Stat(pubPath)
	if err != nil {
		t.Fatalf("Failed to stat public key: %v", err)
	}

	pubMode := pubInfo.Mode() & os.ModePerm
	expectedPubMode := os.FileMode(0644)
	if pubMode != expectedPubMode {
		t.Errorf("Public key permissions mismatch: got %#o, want %#o", pubMode, expectedPubMode)
	}
}

// TestLoadKeyPairNonExistent tests loading a non-existent key pair.
func TestLoadKeyPairNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	keyPath := filepath.Join(tmpDir, ".ssh", "nonexistent")

	_, err = LoadKeyPair(keyPath)
	if err == nil {
		t.Error("Expected error when loading non-existent key pair, but got nil")
	}
}

// TestLoadOrGenerateKeyPair_Generate tests generating when no key pair exists.
func TestLoadOrGenerateKeyPair_Generate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	keyPath := filepath.Join(tmpDir, ".ssh", constant.DefaultKeyName)

	// Load or generate should create a new key pair
	kp, err := LoadOrGenerateKeyPair(keyPath)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeyPair failed: %v", err)
	}

	// Verify key pair was created
	if kp == nil {
		t.Error("LoadOrGenerateKeyPair returned nil key pair")
	}

	// Verify files were saved
	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("Private key was not saved: %v", err)
	}
	if _, err := os.Stat(keyPath + ".pub"); err != nil {
		t.Errorf("Public key was not saved: %v", err)
	}
}

// TestLoadOrGenerateKeyPair_Load tests loading when key pair exists.
func TestLoadOrGenerateKeyPair_Load(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	keyPath := filepath.Join(tmpDir, ".ssh", constant.DefaultKeyName)

	// First call should generate
	kp1, err := LoadOrGenerateKeyPair(keyPath)
	if err != nil {
		t.Fatalf("First LoadOrGenerateKeyPair failed: %v", err)
	}

	// Second call should load the existing key pair
	kp2, err := LoadOrGenerateKeyPair(keyPath)
	if err != nil {
		t.Fatalf("Second LoadOrGenerateKeyPair failed: %v", err)
	}

	// Both should have the same fingerprint (idempotency check)
	if kp1.Fingerprint != kp2.Fingerprint {
		t.Error("Fingerprints don't match - key pair was regenerated instead of loaded")
	}
}

// TestComputeFingerprint tests that fingerprint is consistent.
func TestComputeFingerprint(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Compute fingerprint again from public key
	fp2 := computeFingerprint(kp.PublicKey)

	if kp.Fingerprint != fp2 {
		t.Error("Fingerprint computation is not consistent")
	}
}

// TestDefaultKeyPath tests the default key path.
func TestDefaultKeyPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nvair-ssh-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	path, err := DefaultKeyPath()
	if err != nil {
		t.Fatalf("DefaultKeyPath failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, ".ssh", constant.DefaultKeyName)
	if path != expectedPath {
		t.Errorf("DefaultKeyPath mismatch: got %q, want %q", path, expectedPath)
	}
}
