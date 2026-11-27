package main

// The scheduler image is FROM scratch and has no zoneinfo; embed it so TZ works.
import _ "time/tzdata"

// It has no CA store either; these roots are used only where the system has none.
import _ "golang.org/x/crypto/x509roots/fallback"

// version is set at release build time with -ldflags "-X main.version=<semver>".
var version = "dev"

func main() {
	Main()
}
