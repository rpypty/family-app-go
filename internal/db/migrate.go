package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const migrationsDirName = "migrations"

func Migrate(db *gorm.DB) error {
	path, err := findMigrationsDir(migrationsDirName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if err := ensureSchemaMigrations(db); err != nil {
		return err
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".sql") {
			files = append(files, name)
		}
	}

	sort.Strings(files)

	for _, name := range files {
		applied, err := isMigrationApplied(db, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		contents, err := os.ReadFile(filepath.Join(path, name))
		if err != nil {
			return err
		}

		sql := strings.TrimSpace(string(contents))
		if sql == "" {
			continue
		}

		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		if err := recordMigration(db, name); err != nil {
			return err
		}
	}

	return nil
}

func ensureSchemaMigrations(db *gorm.DB) error {
	return db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`).Error
}

func isMigrationApplied(db *gorm.DB, name string) (bool, error) {
	var count int64
	if err := db.Raw("SELECT COUNT(1) FROM schema_migrations WHERE filename = ?", name).Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func recordMigration(db *gorm.DB, name string) error {
	return db.Exec("INSERT INTO schema_migrations (filename, applied_at) VALUES (?, ?)", name, time.Now().UTC()).Error
}

func findMigrationsDir(dirName string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(dir, dirName)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", os.ErrNotExist
}
