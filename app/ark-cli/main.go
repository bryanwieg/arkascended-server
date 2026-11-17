package main

// version is set at release build time with -ldflags "-X main.version=<semver>".
var version = "dev"

func main() {
	Main()
}
