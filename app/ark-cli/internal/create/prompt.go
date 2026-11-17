package create

import (
	"ark-cli/internal/core"
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

func promptNonEmpty(reader *bufio.Reader, label, fallback, current string) (string, error) {
	defaultValue := fallback
	if current != "" {
		defaultValue = current
	}
	for {
		value, err := promptOptional(reader, fmt.Sprintf("%s [%s]", label, defaultValue))
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return defaultValue, nil
		}
		return value, nil
	}
}

func promptRequired(reader *bufio.Reader, label string) (string, error) {
	for {
		value, err := promptOptional(reader, label)
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			core.Warnln("Value is required.")
			continue
		}
		return value, nil
	}
}

func promptOptional(reader *bufio.Reader, label string) (string, error) {
	core.Warnf("%s: ", label)
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, os.ErrClosed) {
		return "", err
	}
	return strings.TrimSpace(value), nil
}
