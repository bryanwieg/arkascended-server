package update

// UpdateAction describes what should happen for a plan row.
type UpdateAction string

const (
	ActionUpdate UpdateAction = "update"
	ActionSkip   UpdateAction = "skip"
	ActionWarn   UpdateAction = "warn"
)

// PlanRow is a single row in the update plan table.
type PlanRow struct {
	Instance       string
	CurrentImage   string
	CurrentVersion string
	TargetBuildID  string // Steam's current build
	TargetVersion  string // known only once that build's image exists
	Action         UpdateAction
	Message        string
}

// LocalCheckResult holds the result of a local image update check.
type LocalCheckResult struct {
	Image           string
	CurrentVersion  string
	CurrentBuildID  string
	LatestBuildID   string
	UpdateAvailable bool
	CheckFailed     bool
	Message         string
}

// ImageVersionInfo holds parsed version metadata from an image's labels.
type ImageVersionInfo struct {
	Image   string
	Version string
	BuildID string
	Recipe  string // arkcontainer.RecipeID of the ark-cli that built it; empty before recipe labels
}
