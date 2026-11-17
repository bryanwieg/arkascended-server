package compose

import _ "embed"

//go:embed assets/docker-compose.defaults.yaml
var defaultComposeTemplate string
