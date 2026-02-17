package app

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
)

func LoadEnvFiles(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}

	preExisting := map[string]bool{}
	for _, entry := range os.Environ() {
		if idx := strings.Index(entry, "="); idx > 0 {
			preExisting[entry[:idx]] = true
		}
	}

	merged := map[string]string{}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}

		f, err := os.Open(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("open env file %s: %w", path, err)
		}

		scanner := bufio.NewScanner(f)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			key, value, ok, err := parseEnvLine(scanner.Text())
			if err != nil {
				_ = f.Close()
				return fmt.Errorf("parse %s:%d: %w", path, lineNo, err)
			}
			if ok {
				merged[key] = value
			}
		}
		if err := scanner.Err(); err != nil {
			_ = f.Close()
			return fmt.Errorf("scan env file %s: %w", path, err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("close env file %s: %w", path, err)
		}
	}

	for key, value := range merged {
		if preExisting[key] {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s: %w", key, err)
		}
	}

	return nil
}

func parseEnvLine(line string) (key, value string, ok bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}

	if strings.HasPrefix(line, "export ") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	}

	sep := strings.Index(line, "=")
	if sep <= 0 {
		return "", "", false, fmt.Errorf("invalid assignment")
	}

	key = strings.TrimSpace(line[:sep])
	if !validEnvKey(key) {
		return "", "", false, fmt.Errorf("invalid key %q", key)
	}

	raw := strings.TrimSpace(line[sep+1:])
	value, err = parseEnvValue(raw)
	if err != nil {
		return "", "", false, err
	}

	return key, value, true, nil
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if i == 0 {
			if !(r == '_' || unicode.IsLetter(r)) {
				return false
			}
			continue
		}
		if !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func parseEnvValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}

	if strings.HasPrefix(raw, "\"") {
		i, err := findClosingQuote(raw, '"')
		if err != nil {
			return "", err
		}
		rest := strings.TrimSpace(raw[i+1:])
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return "", fmt.Errorf("unexpected characters after quoted value")
		}
		v, err := strconv.Unquote(raw[:i+1])
		if err != nil {
			return "", fmt.Errorf("invalid quoted value: %w", err)
		}
		return v, nil
	}

	if strings.HasPrefix(raw, "'") {
		i := strings.Index(raw[1:], "'")
		if i < 0 {
			return "", fmt.Errorf("unterminated single-quoted value")
		}
		i++
		rest := strings.TrimSpace(raw[i+1:])
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return "", fmt.Errorf("unexpected characters after quoted value")
		}
		return raw[1:i], nil
	}

	return strings.TrimSpace(stripInlineComment(raw)), nil
}

func findClosingQuote(s string, quote byte) (int, error) {
	escape := false
	for i := 1; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		if c == quote {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unterminated quoted value")
}

func stripInlineComment(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != '#' {
			continue
		}
		if i == 0 || unicode.IsSpace(rune(s[i-1])) {
			return s[:i]
		}
	}
	return s
}
