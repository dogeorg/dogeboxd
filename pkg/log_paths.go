package dogeboxd

import (
	"path/filepath"
)

const (
	pupLogPrefix = "pup-"
	jobLogPrefix = "job-"
)

func (c ServerConfig) PupLogFileName(pupID string) string {
	return pupLogPrefix + pupID
}

func (c ServerConfig) PupLogPath(pupID string) string {
	return filepath.Join(c.ContainerLogDir, c.PupLogFileName(pupID))
}

func (c ServerConfig) JobLogFileName(jobID string) string {
	return jobLogPrefix + jobID
}

func (c ServerConfig) JobLogPath(jobID string) string {
	return filepath.Join(c.ContainerLogDir, c.JobLogFileName(jobID))
}
