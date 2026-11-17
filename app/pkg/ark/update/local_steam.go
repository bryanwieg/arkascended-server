package update

import (
	"encoding/json"
	"fmt"
	"strings"
)

// steamAppInfoResponse is the minimal structure needed to extract the public
// branch build ID from the steamcmd.net API response for app 2430930.
type steamAppInfoResponse struct {
	Data map[string]struct {
		Depots struct {
			Branches map[string]struct {
				BuildID string `json:"buildid"`
			} `json:"branches"`
		} `json:"depots"`
	} `json:"data"`
}

// FetchSteamLatestBuildID queries the Steam API for the latest public build ID.
func (s *Service) FetchSteamLatestBuildID() (string, error) {
	if s.FetchText == nil {
		return "", fmt.Errorf("FetchText not configured")
	}
	url := SteamAppInfoURL()
	body, err := s.FetchText(url)
	if err != nil {
		return "", fmt.Errorf("failed to query steam api: %w", err)
	}
	return ParseSteamLatestBuildID(body)
}

// ParseSteamLatestBuildID extracts the build ID from a Steam API JSON response.
func ParseSteamLatestBuildID(body string) (string, error) {
	var resp steamAppInfoResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return "", fmt.Errorf("failed to parse steam api response: %w", err)
	}
	appData, ok := resp.Data["2430930"]
	if !ok {
		return "", fmt.Errorf("steam api response missing app 2430930 data")
	}
	branch, ok := appData.Depots.Branches["public"]
	if !ok {
		return "", fmt.Errorf("steam api response missing public branch")
	}
	buildID := strings.TrimSpace(branch.BuildID)
	if buildID == "" {
		return "", fmt.Errorf("steam api response has empty build id")
	}
	return buildID, nil
}
