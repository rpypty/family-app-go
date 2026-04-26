package receipts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileStore interface {
	Save(ctx context.Context, jobID, fileID string, file UploadedFile) (string, error)
	Load(ctx context.Context, storageKey string) ([]byte, error)
}

type LocalFileStore struct {
	root string
}

func NewLocalFileStore(root string) *LocalFileStore {
	return &LocalFileStore{root: root}
}

func (s *LocalFileStore) Save(_ context.Context, jobID, fileID string, file UploadedFile) (string, error) {
	key := filepath.Join(jobID, fileID)
	path := filepath.Join(s.root, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create receipt file directory: %w", err)
	}
	if err := os.WriteFile(path, file.Data, 0o600); err != nil {
		return "", fmt.Errorf("write receipt file: %w", err)
	}
	return key, nil
}

func (s *LocalFileStore) Load(_ context.Context, storageKey string) ([]byte, error) {
	cleanKey := filepath.Clean(storageKey)
	if filepath.IsAbs(cleanKey) || cleanKey == ".." || strings.HasPrefix(cleanKey, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid receipt file storage key")
	}
	path := filepath.Join(s.root, cleanKey)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read receipt file: %w", err)
	}
	return data, nil
}
