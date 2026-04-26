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
	Delete(ctx context.Context, storageKey string) error
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

func (s *LocalFileStore) Delete(_ context.Context, storageKey string) error {
	cleanKey := filepath.Clean(storageKey)
	if filepath.IsAbs(cleanKey) || cleanKey == ".." || strings.HasPrefix(cleanKey, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid receipt file storage key")
	}

	path := filepath.Join(s.root, cleanKey)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("delete receipt file: %w", err)
	}

	rootClean := filepath.Clean(s.root)
	for dir := filepath.Dir(path); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		if dir == rootClean {
			break
		}
		if err := os.Remove(dir); err != nil {
			break
		}
	}

	return nil
}
