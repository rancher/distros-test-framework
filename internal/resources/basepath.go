package resources

import (
	"path/filepath"
	"runtime"
)

// BasePath returns the base path of the project.
func BasePath() string {
	_, callerFilePath, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(callerFilePath), "../..")
}
