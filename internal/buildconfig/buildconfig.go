package buildconfig

// Build-time variables injected via ldflags
var (
	version = "dev"
	commit  = "unknown"
)

// Version returns the build version
func Version() string {
	return version
}

// Commit returns the git commit hash
func Commit() string {
	return commit
}

// VersionInfo returns full version information
func VersionInfo() map[string]string {
	return map[string]string{
		"version": version,
		"commit":  commit,
	}
}
