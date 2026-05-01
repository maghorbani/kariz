package artifact

import (
	"fmt"
	"os"

	"github.com/kariz/kariz/internal/models"
)

// NewArtifactStore creates the appropriate ArtifactStore implementation based on
// the command's ArtifactDestConfig. For "local" or nil config, it returns a
// LocalArtifactStore. For "minio" or "s3", it returns an S3ArtifactStore.
func NewArtifactStore(destConfig *models.ArtifactDestConfig, localBasePath string) (ArtifactStore, error) {
	if destConfig == nil || destConfig.Type == models.DestLocal || destConfig.Type == "" {
		return NewLocalArtifactStore(localBasePath), nil
	}

	switch destConfig.Type {
	case models.DestMinIO, models.DestS3:
		return newS3StoreFromConfig(destConfig)
	default:
		return nil, fmt.Errorf("unsupported artifact destination type: %s", destConfig.Type)
	}
}

// newS3StoreFromConfig builds an S3ArtifactStore from an ArtifactDestConfig.
// Credentials are resolved from the environment variable named in CredentialRef,
// which should contain "ACCESS_KEY:SECRET_KEY".
func newS3StoreFromConfig(cfg *models.ArtifactDestConfig) (*S3ArtifactStore, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("artifact destination endpoint is required for %s storage", cfg.Type)
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("artifact destination bucket is required for %s storage", cfg.Type)
	}

	accessKey, secretKey := resolveCredentials(cfg.CredentialRef)

	useSSL := cfg.Type == models.DestS3 // default: SSL for S3, no SSL for MinIO

	return NewS3ArtifactStore(
		cfg.Endpoint,
		cfg.Bucket,
		cfg.PathPrefix,
		cfg.Region,
		accessKey,
		secretKey,
		useSSL,
	)
}

// resolveCredentials reads S3 credentials from environment variables.
// If credentialRef is provided, it looks for {credentialRef}_ACCESS_KEY and
// {credentialRef}_SECRET_KEY. Otherwise falls back to AWS_ACCESS_KEY_ID and
// AWS_SECRET_ACCESS_KEY.
func resolveCredentials(credentialRef string) (accessKey, secretKey string) {
	if credentialRef != "" {
		accessKey = os.Getenv(credentialRef + "_ACCESS_KEY")
		secretKey = os.Getenv(credentialRef + "_SECRET_KEY")
		if accessKey != "" && secretKey != "" {
			return accessKey, secretKey
		}
	}
	// Fallback to standard AWS env vars.
	accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	return accessKey, secretKey
}
