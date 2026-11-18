// Command recipe-id prints arkcontainer.RecipeID(), so .github/build-image.sh tags
// and labels its images exactly like ark-cli's local builds.
package main

import (
	"os"

	arkcontainer "ark-container"
)

func main() {
	_, _ = os.Stdout.WriteString(arkcontainer.RecipeID() + "\n") // a build tool, not CLI output
}
