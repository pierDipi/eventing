package prow

import (
	"log"
	"os"
	"strings"
)

const (
	// ArtifactsDir is the dir containing artifacts
	ArtifactsDir = "artifacts"
)

// GetLocalArtifactsDir gets the artifacts directory where prow looks for artifacts.
// By default, it will look at the env var ARTIFACTS.
func GetLocalArtifactsDir() string {
	dir := os.Getenv("ARTIFACTS")
	if dir == "" {
		log.Printf("Env variable ARTIFACTS not set. Using %s instead.", ArtifactsDir)
		dir = ArtifactsDir
	}
	return dir
}

// IsCI returns whether the current environment is a CI environment.
func IsCI() bool {
	return strings.EqualFold(os.Getenv("CI"), "true")
}

