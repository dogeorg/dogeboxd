package dogeboxd

import (
	"bufio"
	"bytes"
	"os/exec"
	"time"
)

type actionLogger struct {
	Job       Job
	PupID     string
	Queued    bool
	dbx       Dogeboxd
	Step      string
	StepStart time.Time
	Progress  int
}

func NewActionLogger(j Job, pupID string, queued bool, dbx Dogeboxd) *actionLogger {
	l := actionLogger{
		Job:    j,
		PupID:  pupID,
		Queued: queued,
		dbx:    dbx,
	}
	return &l
}

func (t *actionLogger) updateStep(step string) {
	if t.Step != step {
		t.Step = step
		t.StepStart = time.Now()
	}
}

func (t *actionLogger) log(step string, msg string, err bool) {
	t.updateStep(step)
	p := ActionProgress{
		ActionID:   t.Job.ID,
		PupID:      t.PupID,
		Progress:   t.Progress,
		Step:       step,
		Msg:        msg,
		Error:      err,
		Queued:     t.Queued,
		StepTaken:  time.Since(t.StepStart),
		TotalTaken: time.Since(t.Job.Start),
	}
	t.dbx.sendProgress(p)
}

func (t *actionLogger) Log(step string, msg string) {
	t.log(step, msg, false)
}

func (t *actionLogger) Err(step string, msg string) {
	t.log(step, msg, true)
}

func (t *actionLogger) LogCmd(step string, cmd *exec.Cmd) {
	cmd.Stdout = NewLineWriter(func(s string) {
		t.log(step, s, false)
	})

	cmd.Stderr = NewLineWriter(func(s string) {
		t.log(step, s, true)
	})
}

type LineWriter struct {
	receiver func(string)
	buf      bytes.Buffer
}

func (t *actionLogger) Progress(p int) *actionLogger {
	t.Progress = p
	return t
}

// implements io.Writer and calls a function for each line
func NewLineWriter(receiver func(string)) *LineWriter {
	return &LineWriter{receiver: receiver}
}

func (t *LineWriter) Write(p []byte) (int, error) {
	scanner := bufio.NewScanner(bytes.NewReader(p))
	scanner.Split(bufio.ScanLines)

	var lastLine bytes.Buffer

	for scanner.Scan() {
		lastLine.Write(t.buf.Bytes())
		t.buf.Reset()
		lastLine.Write(scanner.Bytes())

		t.receiver(lastLine.String())
		lastLine.Reset()
	}

	if len(p) > 0 && p[len(p)-1] != '\n' {
		t.buf.Write(p[scanner.Err() != nil:])
	}

	return len(p), scanner.Err()
}
