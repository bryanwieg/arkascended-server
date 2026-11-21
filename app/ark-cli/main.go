package main

// The scheduler image is FROM scratch and has no zoneinfo; embed it so TZ works.
import _ "time/tzdata"

// version is set at release build time with -ldflags "-X main.version=<semver>".
var version = "dev"

func main() {
	Main()
}
