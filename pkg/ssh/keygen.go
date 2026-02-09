package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"github.com/unifabric-io/nvair-cli/pkg/constant"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"golang.org/x/crypto/ssh"
)

// KeyPair represents an Ed25519 SSH key pair.
type KeyPair struct {
	PrivateKey  []byte // PEM-encoded private key
	PublicKey   []byte // OpenSSH format public key
	Fingerprint string // Base64-encoded SHA256 fingerprint
}

// DefaultKeyPath returns the default SSH key path for nvair.
// Default: ~/.ssh/nvair.unifabric.io
func DefaultKeyPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".ssh", constant.DefaultKeyName), nil
}

// GenerateKeyPair generates a new Ed25519 SSH key pair.
// Returns the KeyPair or an error.
func GenerateKeyPair() (*KeyPair, error) {
	logging.Verbose("GenerateKeyPair: Starting Ed25519 key pair generation")

	// Generate Ed25519 key pair
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		logging.Verbose("GenerateKeyPair: Failed to generate Ed25519 key pair: %v", err)
		return nil, fmt.Errorf("failed to generate Ed25519 key pair: %w", err)
	}

	logging.Verbose("GenerateKeyPair: Ed25519 key pair generated successfully")

	// Marshal private key in OpenSSH format (pass the raw private key, not the signer)
	pemBlock, err := ssh.MarshalPrivateKey(privateKey, "")
	if err != nil {
		logging.Verbose("GenerateKeyPair: Failed to marshal private key: %v", err)
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	// Encode PEM block to bytes
	privateKeyPEM := pem.EncodeToMemory(pemBlock)
	logging.Verbose("GenerateKeyPair: Private key encoded in PEM format")

	// Encode public key in OpenSSH format
	pubKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		logging.Verbose("GenerateKeyPair: Failed to create public key: %v", err)
		return nil, fmt.Errorf("failed to create public key: %w", err)
	}

	publicKeyBytes := ssh.MarshalAuthorizedKey(pubKey)
	logging.Verbose("GenerateKeyPair: Public key encoded in OpenSSH format")

	// Compute SHA256 fingerprint
	fingerprint := computeFingerprint(publicKeyBytes)
	logging.Verbose("GenerateKeyPair: SSH key fingerprint computed: %s", fingerprint)

	return &KeyPair{
		PrivateKey:  privateKeyPEM,
		PublicKey:   publicKeyBytes,
		Fingerprint: fingerprint,
	}, nil
}

// SaveKeyPair saves the key pair to disk with appropriate permissions.
// Private key is saved with 0600, public key with 0644.
func (kp *KeyPair) SaveKeyPair(basePath string) error {
	logging.Verbose("SaveKeyPair: Starting key pair save to %s", basePath)

	// Create the .ssh directory if it doesn't exist
	sshDir := filepath.Dir(basePath)
	logging.Verbose("SaveKeyPair: Creating SSH directory: %s", sshDir)
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		logging.Verbose("SaveKeyPair: Failed to create .ssh directory: %v", err)
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Write private key with 0600 permissions
	privateKeyPath := basePath
	logging.Verbose("SaveKeyPair: Writing private key with 0600 permissions to %s", privateKeyPath)
	if err := os.WriteFile(privateKeyPath, kp.PrivateKey, 0600); err != nil {
		logging.Verbose("SaveKeyPair: Failed to write private key: %v", err)
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Write public key with 0644 permissions
	publicKeyPath := basePath + ".pub"
	logging.Verbose("SaveKeyPair: Writing public key with 0644 permissions to %s", publicKeyPath)
	if err := os.WriteFile(publicKeyPath, kp.PublicKey, 0644); err != nil {
		// Clean up private key if public key write fails
		logging.Verbose("SaveKeyPair: Failed to write public key, cleaning up private key: %v", err)
		os.Remove(privateKeyPath)
		return fmt.Errorf("failed to write public key: %w", err)
	}

	logging.Verbose("SaveKeyPair: Key pair saved successfully")
	return nil
}

// LoadKeyPair loads an existing key pair from disk.
// If the files don't exist, returns a not-found error.
func LoadKeyPair(basePath string) (*KeyPair, error) {
	logging.Verbose("LoadKeyPair: Loading key pair from %s", basePath)

	privateKeyPath := basePath
	publicKeyPath := basePath + ".pub"

	// Check if files exist
	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		logging.Verbose("LoadKeyPair: Private key not found at %s", privateKeyPath)
		return nil, fmt.Errorf("private key not found at %s: %w", privateKeyPath, err)
	}

	if _, err := os.Stat(publicKeyPath); os.IsNotExist(err) {
		logging.Verbose("LoadKeyPair: Public key not found at %s", publicKeyPath)
		return nil, fmt.Errorf("public key not found at %s: %w", publicKeyPath, err)
	}

	logging.Verbose("LoadKeyPair: Key pair files found, reading contents")

	// Read files
	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		logging.Verbose("LoadKeyPair: Failed to read private key: %v", err)
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	publicKey, err := os.ReadFile(publicKeyPath)
	if err != nil {
		logging.Verbose("LoadKeyPair: Failed to read public key: %v", err)
		return nil, fmt.Errorf("failed to read public key: %w", err)
	}

	// Compute fingerprint
	fingerprint := computeFingerprint(publicKey)
	logging.Verbose("LoadKeyPair: Key pair loaded successfully, fingerprint: %s", fingerprint)

	return &KeyPair{
		PrivateKey:  privateKey,
		PublicKey:   publicKey,
		Fingerprint: fingerprint,
	}, nil
}

// LoadOrGenerateKeyPair loads an existing key pair from disk.
// If no key pair exists, generates a new one and saves it.
func LoadOrGenerateKeyPair(basePath string) (*KeyPair, error) {
	logging.Verbose("LoadOrGenerateKeyPair: Attempting to load or generate key pair at %s", basePath)

	// Try to load existing key pair
	kp, err := LoadKeyPair(basePath)
	if err == nil {
		logging.Verbose("LoadOrGenerateKeyPair: Successfully loaded existing key pair")
		return kp, nil
	}

	// Check if this is a "not found" error (vs. other read errors)
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		// Key pair doesn't exist, generate new one
		logging.Verbose("LoadOrGenerateKeyPair: Key pair not found, generating new one")
		kp, err := GenerateKeyPair()
		if err != nil {
			logging.Verbose("LoadOrGenerateKeyPair: Failed to generate key pair: %v", err)
			return nil, err
		}

		// Save the new key pair
		logging.Verbose("LoadOrGenerateKeyPair: Saving newly generated key pair")
		if err := kp.SaveKeyPair(basePath); err != nil {
			logging.Verbose("LoadOrGenerateKeyPair: Failed to save key pair: %v", err)
			return nil, err
		}

		logging.Verbose("LoadOrGenerateKeyPair: New key pair generated and saved successfully")
		return kp, nil
	}

	// Key pair files are partially present or have other issues
	logging.Verbose("LoadOrGenerateKeyPair: Unable to load or generate key pair: %v", err)
	return nil, fmt.Errorf("unable to load or generate key pair: %w", err)
}

// computeFingerprint computes the SHA256 fingerprint of the public key.
// The fingerprint is base64-encoded, matching the API format.
func computeFingerprint(publicKey []byte) string {
	hash := sha256.Sum256(publicKey)
	return base64.StdEncoding.EncodeToString(hash[:])
}
