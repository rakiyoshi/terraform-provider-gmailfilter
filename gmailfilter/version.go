package gmailfilter

import "fmt"

var (
	// Version app version
	Version = "1.0.1"
	// Revision git commit short commithash
	Revision = "xxxxxx" // set on build
)

// FullVersion return sackerel full version text
func FullVersion() string {
	return fmt.Sprintf("%s, build %s", Version, Revision)
}
