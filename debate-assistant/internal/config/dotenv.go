// Package config contains small, dependency-free configuration helpers.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LoadDotEnv searches the current working directory and each parent directory
// for a .env file. Existing non-empty process environment variables take
// precedence. A missing .env file is not an error.
func LoadDotEnv() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, ".env")
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() {
			if err := loadFile(candidate); err != nil {
				return candidate, err
			}
			return candidate, nil
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return candidate, fmt.Errorf("inspect .env file: %w", statErr)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", nil
}

func loadFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open .env: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\uFEFF"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok, parseErr := parseLine(line)
		if parseErr != nil {
			return fmt.Errorf(".env line %d: %w", lineNumber, parseErr)
		}
		if !ok {
			continue
		}
		if current := strings.TrimSpace(os.Getenv(key)); current != "" {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s from .env: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read .env: %w", err)
	}
	return nil
}

func parseLine(line string) (key, value string, ok bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}
	if strings.HasPrefix(line, "export ") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	}
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false, fmt.Errorf("expected KEY=value")
	}
	key = strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", false, fmt.Errorf("environment variable name is empty")
	}
	for i, r := range key {
		valid := r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (i > 0 && r >= '0' && r <= '9')
		if !valid {
			return "", "", false, fmt.Errorf("invalid environment variable name %q", key)
		}
	}

	raw := strings.TrimSpace(parts[1])
	if raw == "" {
		return key, "", true, nil
	}
	if raw[0] == '"' {
		end := closingQuote(raw, '"')
		if end < 0 {
			return "", "", false, fmt.Errorf("unterminated double-quoted value")
		}
		quoted := raw[:end+1]
		value, err = strconv.Unquote(quoted)
		if err != nil {
			return "", "", false, fmt.Errorf("invalid quoted value: %w", err)
		}
		if trailing := strings.TrimSpace(raw[end+1:]); trailing != "" && !strings.HasPrefix(trailing, "#") {
			return "", "", false, fmt.Errorf("unexpected text after quoted value")
		}
		return key, value, true, nil
	}
	if raw[0] == '\'' {
		end := strings.Index(raw[1:], "'")
		if end < 0 {
			return "", "", false, fmt.Errorf("unterminated single-quoted value")
		}
		end++
		value = raw[1:end]
		if trailing := strings.TrimSpace(raw[end+1:]); trailing != "" && !strings.HasPrefix(trailing, "#") {
			return "", "", false, fmt.Errorf("unexpected text after quoted value")
		}
		return key, value, true, nil
	}
	if index := strings.Index(raw, " #"); index >= 0 {
		raw = raw[:index]
	}
	return key, strings.TrimSpace(raw), true, nil
}

func closingQuote(value string, quote byte) int {
	escaped := false
	for i := 1; i < len(value); i++ {
		if escaped {
			escaped = false
			continue
		}
		if value[i] == '\\' && quote == '"' {
			escaped = true
			continue
		}
		if value[i] == quote {
			return i
		}
	}
	return -1
}
