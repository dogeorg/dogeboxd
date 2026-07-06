package web

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

const defaultLogTailLimit = 1000

type logTailResponse struct {
	Lines        []string `json:"lines"`
	ResumeToken  *string  `json:"resumeToken,omitempty"`
	OlderCursor  *string  `json:"olderCursor,omitempty"`
	HasMoreOlder bool     `json:"hasMoreOlder"`
}

func (t api) downloadPupLog(w http.ResponseWriter, r *http.Request) {
	pupID := r.PathValue("PupID")
	if pupID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Missing pup id")
		return
	}

	if pupID == "dbx" || pupID == "dkm" {
		sendErrorResponse(w, http.StatusBadRequest, "Journal logs cannot be downloaded from this endpoint")
		return
	}

	if _, _, err := t.pups.GetPup(pupID); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Cannot find pup")
		return
	}

	t.streamLogDownload(w, t.config.PupLogPath(pupID), t.config.PupLogFileName(pupID)+".log")
}

func (t api) downloadJobLog(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("JobID")
	if jobID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Missing job id")
		return
	}

	if _, err := t.dbx.JobManager.GetJob(jobID); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Cannot find job")
		return
	}

	t.streamLogDownload(w, t.config.JobLogPath(jobID), t.config.JobLogFileName(jobID)+".log")
}

func (t api) getPupLogTail(w http.ResponseWriter, r *http.Request) {
	t.getLogTail(w, r, "PupID", t.dbx.GetLogPage)
}

func (t api) getJobLogTail(w http.ResponseWriter, r *http.Request) {
	t.getLogTail(w, r, "JobID", t.dbx.GetJobLogPage)
}

func (t api) getLogTail(
	w http.ResponseWriter,
	r *http.Request,
	pathValue string,
	fetchTail func(string, *string, int) (dogeboxd.LogPage, error),
) {
	logID := r.PathValue(pathValue)
	limit, err := parseLogTailLimit(r)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	before, err := parseLogTailBefore(r)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	page, err := fetchTail(logID, before, limit)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	sendResponse(w, logTailResponse{
		Lines:        page.Lines,
		ResumeToken:  page.ResumeToken,
		OlderCursor:  page.OlderCursor,
		HasMoreOlder: page.HasMoreOlder,
	})
}

func parseLogTailLimit(r *http.Request) (int, error) {
	rawLimit := r.URL.Query().Get("limit")
	if rawLimit == "" {
		return defaultLogTailLimit, nil
	}

	limit, err := strconv.Atoi(rawLimit)
	if err != nil {
		return 0, errors.New("Invalid log tail limit")
	}
	if limit <= 0 {
		return 0, errors.New("Log tail limit must be greater than zero")
	}
	if limit > defaultLogTailLimit {
		return defaultLogTailLimit, nil
	}

	return limit, nil
}

func parseLogTailBefore(r *http.Request) (*string, error) {
	before := r.URL.Query().Get("before")
	if before == "" {
		return nil, nil
	}

	return &before, nil
}
func (t api) streamLogDownload(w http.ResponseWriter, logPath string, downloadName string) {
	logFile, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			sendErrorResponse(w, http.StatusNotFound, "Log file not found")
			return
		}
		sendErrorResponse(w, http.StatusInternalServerError, "Error opening log file")
		return
	}
	defer logFile.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", downloadName))
	w.Header().Set("Cache-Control", "no-store")

	if _, err := io.Copy(w, logFile); err != nil {
		log.Printf("Error streaming log file %s: %v", logPath, err)
	}
}
