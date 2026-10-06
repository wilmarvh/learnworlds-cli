// Package skills embeds the agent skill shipped with the CLI.
package skills

import _ "embed"

//go:embed learnworlds/SKILL.md
var LearnWorlds string
