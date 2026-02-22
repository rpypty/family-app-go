package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"family-app-go/pkg/logger"
)

const dotenvFilename = ".env"

func loadDotEnv(log logger.Logger) error {
	path, err := findDotEnv(dotenvFilename)
	if err != nil {
		return err
	}

	loaded, skipped, err := parseDotEnv(path)
	if err != nil {
		return err
	}

	log.Info("dotenv: loaded variables", "count", loaded, "path", path)
	if skipped > 0 {
		log.Info("dotenv: skipped variables already set in env", "count", skipped)
	}

	return nil
}

func findDotEnv(filename string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(dir, filename)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
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

func parseDotEnv(path string) (int, int, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	loaded := 0
	skipped := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := splitKeyValue(line)
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			skipped++
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return loaded, skipped, err
		}
		loaded++
	}

	if err := scanner.Err(); err != nil {
		return loaded, skipped, err
	}

	return loaded, skipped, nil
}

func splitKeyValue(line string) (string, string, bool) {
	idx := strings.Index(line, "=")
	if idx == -1 {
		return "", "", false
	}

	key := strings.TrimSpace(line[:idx])
	if key == "" {
		return "", "", false
	}

	value := strings.TrimSpace(line[idx+1:])
	if value == "" {
		return key, "", true
	}

	quoted := len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[0] == value[len(value)-1]
	if quoted {
		if value[0] == '"' {
			unquoted, err := strconv.Unquote(value)
			if err == nil {
				return key, unquoted, true
			}
		}
		return key, value[1 : len(value)-1], true
	}

	return key, stripInlineComment(value), true
}

func stripInlineComment(value string) string {
	for i := 1; i < len(value); i++ {
		if value[i] == '#' {
			prev := value[i-1]
			if prev == ' ' || prev == '\t' {
				return strings.TrimSpace(value[:i-1])
			}
		}
	}
	return value
}
