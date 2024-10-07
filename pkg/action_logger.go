package dogeboxd

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"time"
)

type ActionLogger interface {
	Progress(p int)
	Step(step string) SubLogger
}

type SubLogger interface {
	Log(msg string)
	Logf(msg string, a ...any)
	Err(msg string)
	Errf(msg string, a ...any)
	Progress(p int) SubLogger
	LogCmd(cmd *exec.Cmd)
}

type actionLogger struct {
	Job      Job
	PupID    string
	dbx      Dogeboxd
	Steps    map[string]*stepLogger
	progress int
}

func NewActionLogger(j Job, pupID string, dbx Dogeboxd) *actionLogger {
	l := actionLogger{
		Job:   j,
		PupID: pupID,
		dbx:   dbx,
		Steps: map[string]*stepLogger{},
	}
	return &l
}

func (t *actionLogger) Progress(p int) *actionLogger {
	t.progress = p
	return t
}

func (t *actionLogger) Step(step string) *stepLogger {
	s, ok := t.Steps[step]
	if !ok {
		t.Steps[step] = &stepLogger{t, step, 0, time.Now()}
		s = t.Steps[step]
	}
	return s
}

type stepLogger struct {
	l        *actionLogger
	step     string
	progress int
	start    time.Time
}

func (t *stepLogger) log(msg string, err bool) {
	p := ActionProgress{
		ActionID:  t.l.Job.ID,
		PupID:     t.l.PupID,
		Progress:  t.progress,
		Step:      t.step,
		Msg:       msg,
		Error:     err,
		StepTaken: time.Since(t.start),
	}
	symbol := "✔️"
	if p.Error {
		symbol = "⁉️"
	}
	fmt.Printf("%s [%s:%s](%.2fs|%d%%): %s\n", symbol, p.ActionID, p.Step, p.StepTaken.Seconds(), p.Progress, p.Msg)
	t.l.dbx.sendProgress(p)
}

func (t *stepLogger) Progress(p int) SubLogger {
	t.progress = p
	return t
}

func (t *stepLogger) Log(msg string) {
	t.log(msg, false)
}

func (t *stepLogger) Logf(msg string, a ...any) {
	t.log(fmt.Sprintf(msg, a...), false)
}

func (t *stepLogger) Err(msg string) {
	t.log(msg, true)
}

func (t *stepLogger) Errf(msg string, a ...any) {
	t.log(fmt.Sprintf(msg, a...), true)
}

func (t *stepLogger) LogCmd(cmd *exec.Cmd) {
	cmd.Stdout = NewLineWriter(func(s string) {
		t.log(s, false)
	})

	cmd.Stderr = NewLineWriter(func(s string) {
		t.log(s, true)
	})
}

type ConsoleSubLogger struct {
	PupID    string
	step     string
	progress int
	start    time.Time
}

func NewConsoleSubLogger(pupID string, step string) *ConsoleSubLogger {
	l := ConsoleSubLogger{
		PupID:    pupID,
		step:     step,
		progress: 0,
		start:    time.Now(),
	}
	return &l
}

func (t *ConsoleSubLogger) log(msg string, err bool) {
	symbol := "✔️"
	if err {
		symbol = "⁉️"
	}
	fmt.Printf("%s [%s:%s](%.2fs|%d%%): %s\n", symbol, t.PupID, t.step, time.Since(t.start).Seconds(), t.progress, msg)
}

func (t *ConsoleSubLogger) Progress(p int) SubLogger {
	t.progress = p
	return t
}

func (t *ConsoleSubLogger) Log(msg string) {
	t.log(msg, false)
}

func (t *ConsoleSubLogger) Logf(msg string, a ...any) {
	t.log(fmt.Sprintf(msg, a...), false)
}

func (t *ConsoleSubLogger) Err(msg string) {
	t.log(msg, true)
}

func (t *ConsoleSubLogger) Errf(msg string, a ...any) {
	t.log(fmt.Sprintf(msg, a...), true)
}

func (t *ConsoleSubLogger) LogCmd(cmd *exec.Cmd) {
	cmd.Stdout = NewLineWriter(func(s string) {
		t.log(s, false)
	})

	cmd.Stderr = NewLineWriter(func(s string) {
		t.log(s, true)
	})
}

type LineWriter struct {
	receiver func(string)
	buf      bytes.Buffer
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
		if scanner.Err() == nil {
			t.buf.Write(p)
		} else {
			t.buf.Write(p[:len(p)-len(scanner.Bytes())])
		}
	}
	return len(p), scanner.Err()
}
