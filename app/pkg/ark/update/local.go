package update

import (
	"context"
	"fmt"
)

// CheckLocalUpdate checks a local image for available updates via Steam API.
func (s *Service) CheckLocalUpdate(ctx context.Context, image string) (LocalCheckResult, error) {
	result := LocalCheckResult{Image: image}

	info, err := s.InspectImageVersionInfo(ctx, image)
	if err != nil {
		result.CheckFailed = true
		result.Message = fmt.Sprintf("failed to inspect image: %v", err)
		return result, nil
	}
	result.CurrentVersion = info.Version
	result.CurrentBuildID = info.BuildID

	if result.CurrentBuildID == "" {
		result.CheckFailed = true
		result.Message = "local image build id label missing"
		return result, nil
	}

	latestBuildID, err := s.FetchSteamLatestBuildID()
	if err != nil {
		result.CheckFailed = true
		result.Message = fmt.Sprintf("steam api check failed: %v", err)
		return result, nil
	}
	result.LatestBuildID = latestBuildID

	if result.CurrentBuildID != latestBuildID {
		result.UpdateAvailable = true
		result.Message = "update available"
	} else {
		result.Message = "local image is up to date"
	}
	return result, nil
}

// PlanLocalUpdate maps a local check result to a plan row.
func PlanLocalUpdate(instance, image string, check LocalCheckResult) PlanRow {
	row := PlanRow{
		Instance:       instance,
		CurrentImage:   image,
		CurrentVersion: check.CurrentVersion,
		TargetImage:    image,
		TargetVersion:  check.LatestVersion,
		Action:         ActionSkip,
		Message:        check.Message,
	}
	if row.Message == "" {
		row.Message = "local image is up to date"
	}
	if check.CheckFailed {
		row.Action = ActionWarn
		return row
	}
	if check.UpdateAvailable {
		row.Action = ActionUpdate
		if row.Message == "" {
			row.Message = "local rebuild available"
		}
	}
	return row
}
