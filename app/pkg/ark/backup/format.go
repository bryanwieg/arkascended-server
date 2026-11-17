package backup

import (
	"errors"
	"strings"
)

var ArchiveSuffixes = []string{".tar.zst", ".tar.gz", ".tar"}

const FormatValidationError = "Error: format must be tar, tar.gz, or tar.zst."

func IsValidFormat(format string) bool {
	switch format {
	case "tar", "tar.gz", "tar.zst":
		return true
	default:
		return false
	}
}

func ValidateFormat(format string) error {
	if IsValidFormat(format) {
		return nil
	}
	return errors.New(FormatValidationError)
}

func HasArchiveSuffix(name string) bool {
	return DetectArchiveFormat(name) != ""
}

func DetectArchiveFormat(name string) string {
	switch {
	case strings.HasSuffix(name, ".tar.gz"):
		return "tar.gz"
	case strings.HasSuffix(name, ".tar.zst"):
		return "tar.zst"
	case strings.HasSuffix(name, ".tar"):
		return "tar"
	default:
		return ""
	}
}

func TrimArchiveSuffix(name string) string {
	for _, suffix := range ArchiveSuffixes {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}
