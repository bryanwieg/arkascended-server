package compose

import (
	"strconv"
	"strings"
)

// CreateSettings holds the settings for rendering a compose template.
// This type is exported so cmd/create.go can access it.
type CreateSettings struct {
	Instance       string
	MapName        string
	SessionName    string
	AdminPassword  string
	ServerPassword string
	ASAPort        int
	RCONHostPort   int
	TZ             string
	Image          string
}

// RenderTemplate replaces template placeholders with instance settings.
func RenderTemplate(settings CreateSettings) string {
	replacer := strings.NewReplacer(
		"{{INSTANCE}}", settings.Instance,
		"{{MAP_NAME}}", settings.MapName,
		"{{SESSION_NAME}}", settings.SessionName,
		"{{ADMIN_PASSWORD}}", settings.AdminPassword,
		"{{SERVER_PASSWORD}}", settings.ServerPassword,
		"{{ASA_PORT}}", strconv.Itoa(settings.ASAPort),
		"{{RCON_HOST_PORT}}", strconv.Itoa(settings.RCONHostPort),
		"{{TZ}}", settings.TZ,
		"{{IMAGE}}", settings.Image,
	)
	return replacer.Replace(defaultComposeTemplate)
}
