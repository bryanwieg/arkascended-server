package update

import "os"

// DefaultSteamAppInfoURL reports ARK's current Steam build (app 2430930).
const DefaultSteamAppInfoURL = "https://api.steamcmd.net/v1/info/2430930"

// SteamAppInfoURL returns the Steam API URL for build ID checks.
func SteamAppInfoURL() string {
	return envOrDefault("ARK_STEAM_API_URL", DefaultSteamAppInfoURL)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
