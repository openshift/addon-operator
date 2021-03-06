package version

import (
	"fmt"
	"runtime"
	"strconv"
	"time"
)

// https://github.com/golang/go/issues/37369
const (
	empty = "was not build properly"
)

// Values are provided by compile time -ldflags.
var (
	Version   = empty
	Branch    = empty
	Commit    = empty
	BuildDate = empty
)

// Info contains build information supplied during compile time.
type Info struct {
	Version   string    `json:"version"`
	Branch    string    `json:"branch"`
	Commit    string    `json:"commit"`
	BuildDate time.Time `json:"buildTime"`
	GoVersion string    `json:"goVersion"`
	Platform  string    `json:"platform"`
}

// Get returns the build-in version and platform information
func Get() Info {
	v := Info{
		Version:   Version,
		Branch:    Branch,
		Commit:    Commit,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}

	if BuildDate != empty {
		i, err := strconv.ParseInt(BuildDate, 10, 64)
		if err != nil {
			panic(fmt.Errorf("error parsing build time: %w", err))
		}
		v.BuildDate = time.Unix(i, 0).UTC()
	}

	return v
}
