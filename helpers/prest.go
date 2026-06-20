package helpers

var (
	// PrestVersionNumber repesemts prest version.
	PrestVersionNumber = "2.2.1"
	// CommitHash for version
	CommitHash string
)

// PrestReleaseVersion is same as pREST Version.
func PrestReleaseVersion() string {
	return PrestVersionNumber
}
