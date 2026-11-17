package schedule

import (
	"fmt"
	"strings"
)

const ErrRetentionValuesNonNegative = "Error: retention values must be >= 0."

func ErrNotFound(name string) error {
	return fmt.Errorf("Error: schedule not found: %s", name)
}

func ErrUnsupportedType(scheduleType string) error {
	return fmt.Errorf("Error: unsupported schedule type: %s", scheduleType)
}

func ErrValidationFailed(name string, err error) error {
	message := strings.TrimSpace(err.Error())
	message = strings.TrimPrefix(message, "Error: ")
	return fmt.Errorf("Error: schedule %s is invalid: %s", name, message)
}
