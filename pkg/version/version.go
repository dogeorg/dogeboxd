package version

import (
	"github.com/carlmjohnson/versioninfo"
)

/* injected */

var dbxRelease string
var nurHash string

/* ** */

type DBXVersionInfoGit struct {
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

type DBXVersionInfo struct {
	Release string            `json:"release"`
	NurHash string            `json:"nurHash"`
	Git     DBXVersionInfoGit `json:"git"`
}

func GetDBXRelease() *DBXVersionInfo {
	release := dbxRelease
	nurHash := nurHash

	if release == "" {
		release = "unknown"
	}

	if nurHash == "" {
		nurHash = "unknown"
	}

	return &DBXVersionInfo{
		Release: release,
		NurHash: nurHash,
		Git: DBXVersionInfoGit{
			Commit: versioninfo.Revision,
			Dirty:  versioninfo.DirtyBuild,
		},
	}
}
