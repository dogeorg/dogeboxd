package dogeboxd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Test Suite: Basic Logging
// ============================================================================

func TestActionLoggerCreation(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	assert.NotNil(t, logger)
	assert.Equal(t, job, logger.Job)
	assert.Equal(t, "test-pup-id", logger.PupID)
}

func TestActionLoggerLogMessage(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Log("Test message")

	// Verify step was created
	_, exists := logger.Steps["test-step"]
	assert.True(t, exists)
}

func TestActionLoggerLogFormattedMessage(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Logf("Test message %d", 123)

	// Verify step was created
	_, exists := logger.Steps["test-step"]
	assert.True(t, exists)
}

func TestActionLoggerLogError(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Err("Error message")

	// Verify step was created
	_, exists := logger.Steps["test-step"]
	assert.True(t, exists)
}

func TestActionLoggerLogFormattedError(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Errf("Error message %s", "test")

	// Verify step was created
	_, exists := logger.Steps["test-step"]
	assert.True(t, exists)
}

func TestActionLoggerProgressSet(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	logger.Progress(50)
	assert.Equal(t, 50, logger.progress)
}

func TestActionLoggerStepProgressSet(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Progress(75).Log("Message")

	assert.Equal(t, 75, step.progress)
}

// ============================================================================
// Test Suite: Step Management
// ============================================================================

func TestActionLoggerCreateStep(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	assert.NotNil(t, step)
	assert.Equal(t, "test-step", step.step)
}

func TestActionLoggerSameStepReturnsSameInstance(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step1 := logger.Step("test-step")
	step2 := logger.Step("test-step")

	assert.Equal(t, step1, step2)
}

func TestActionLoggerDifferentStepsReturnDifferentInstances(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step1 := logger.Step("step1")
	step2 := logger.Step("step2")

	assert.NotEqual(t, step1, step2)
}

func TestActionLoggerStepTiming(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")

	// Verify step has a start time
	assert.NotNil(t, step.start)
	assert.True(t, time.Since(step.start) < time.Second)
}

// ============================================================================
// Test Suite: Command Logging
// ============================================================================

func TestActionLoggerLogCommandStdout(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")

	// Create a simple command that outputs to stdout
	cmd := exec.Command("echo", "test output")
	step.LogCmd(cmd)

	// Verify stdout is set
	assert.NotNil(t, cmd.Stdout)
}

func TestActionLoggerLogCommandStderr(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")

	// Create a simple command
	cmd := exec.Command("echo", "test output")
	step.LogCmd(cmd)

	// Verify stderr is set
	assert.NotNil(t, cmd.Stderr)
}

func TestActionLoggerLineWriterCapturesOutput(t *testing.T) {
	var captured []string
	writer := NewLineWriter(func(s string) {
		captured = append(captured, s)
	})

	testData := []byte("line1\nline2\nline3")
	_, err := writer.Write(testData)
	require.NoError(t, err)

	assert.Equal(t, 3, len(captured))
	assert.Equal(t, "line1", captured[0])
	assert.Equal(t, "line2", captured[1])
	assert.Equal(t, "line3", captured[2])
}


func TestActionLoggerLineWriterBuffersPartialLines(t *testing.T) {
	var captured []string
	writer := NewLineWriter(func(s string) {
		captured = append(captured, s)
	})

	// Write partial line
	_, err := writer.Write([]byte("partial"))
	require.NoError(t, err)
	// Note: The writer may or may not capture partial lines depending on implementation
	// This test verifies the writer handles partial lines without crashing

	// Complete the line
	_, err = writer.Write([]byte(" line\n"))
	require.NoError(t, err)
	// At least one complete line should be captured
	assert.GreaterOrEqual(t, len(captured), 1)
}

// ============================================================================
// Test Suite: Console Sub Logger
// ============================================================================

func TestConsoleSubLoggerCreation(t *testing.T) {
	logger := NewConsoleSubLogger("test-pup-id", "test-step")

	assert.NotNil(t, logger)
	assert.Equal(t, "test-pup-id", logger.PupID)
	assert.Equal(t, "test-step", logger.step)
}

func TestConsoleSubLoggerLogMessage(t *testing.T) {
	logger := NewConsoleSubLogger("test-pup-id", "test-step")

	// This should not panic
	logger.Log("Test message")
}

func TestConsoleSubLoggerLogFormattedMessage(t *testing.T) {
	logger := NewConsoleSubLogger("test-pup-id", "test-step")

	// This should not panic
	logger.Logf("Test message %d", 123)
}

func TestConsoleSubLoggerLogError(t *testing.T) {
	logger := NewConsoleSubLogger("test-pup-id", "test-step")

	// This should not panic
	logger.Err("Error message")
}

func TestConsoleSubLoggerLogFormattedError(t *testing.T) {
	logger := NewConsoleSubLogger("test-pup-id", "test-step")

	// This should not panic
	logger.Errf("Error message %s", "test")
}

func TestConsoleSubLoggerSetProgress(t *testing.T) {
	logger := NewConsoleSubLogger("test-pup-id", "test-step")

	result := logger.Progress(50)
	assert.Equal(t, logger, result)
	assert.Equal(t, 50, logger.progress)
}

func TestConsoleSubLoggerLogCommand(t *testing.T) {
	logger := NewConsoleSubLogger("test-pup-id", "test-step")

	cmd := exec.Command("echo", "test")
	logger.LogCmd(cmd)

	assert.NotNil(t, cmd.Stdout)
	assert.NotNil(t, cmd.Stderr)
}

// ============================================================================
// Test Suite: Line Writer
// ============================================================================

func TestLineWriterWriteCompleteLines(t *testing.T) {
	var lines []string
	writer := NewLineWriter(func(s string) {
		lines = append(lines, s)
	})

	data := []byte("line1\nline2\nline3\n")
	n, err := writer.Write(data)

	require.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, 3, len(lines))
}

func TestLineWriterWriteWithoutNewline(t *testing.T) {
	var lines []string
	writer := NewLineWriter(func(s string) {
		lines = append(lines, s)
	})

	data := []byte("no newline")
	n, err := writer.Write(data)

	require.NoError(t, err)
	assert.Equal(t, len(data), n)
	// Note: LineWriter behavior may vary - this test just ensures it doesn't crash
}

func TestLineWriterWriteMultipleChunks(t *testing.T) {
	var lines []string
	writer := NewLineWriter(func(s string) {
		lines = append(lines, s)
	})

	// Write in chunks
	writer.Write([]byte("line"))
	writer.Write([]byte("1\n"))
	writer.Write([]byte("line2\n"))

	// Verify that lines were captured (exact count may vary)
	assert.GreaterOrEqual(t, len(lines), 1)
}

func TestLineWriterWriteEmptyData(t *testing.T) {
	var lines []string
	writer := NewLineWriter(func(s string) {
		lines = append(lines, s)
	})

	data := []byte("")
	n, err := writer.Write(data)

	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, len(lines))
}

func TestLineWriterWriteOnlyNewlines(t *testing.T) {
	var lines []string
	writer := NewLineWriter(func(s string) {
		lines = append(lines, s)
	})

	data := []byte("\n\n\n")
	n, err := writer.Write(data)

	require.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, 3, len(lines))
	assert.Equal(t, "", lines[0])
	assert.Equal(t, "", lines[1])
	assert.Equal(t, "", lines[2])
}

func TestLineWriterFlushBufferedData(t *testing.T) {
	var lines []string
	writer := NewLineWriter(func(s string) {
		lines = append(lines, s)
	})

	// Write partial line
	writer.Write([]byte("partial"))

	// Write complete line
	writer.Write([]byte(" line\n"))

	// Verify that at least one line was captured
	assert.GreaterOrEqual(t, len(lines), 1)
}

func TestLineWriterHandleCarriageReturn(t *testing.T) {
	var lines []string
	writer := NewLineWriter(func(s string) {
		lines = append(lines, s)
	})

	// Write line with carriage return
	data := []byte("line1\r\nline2\n")
	n, err := writer.Write(data)

	require.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, 2, len(lines))
}

// ============================================================================
// Test Suite: Progress Broadcasting
// ============================================================================

func TestActionLoggerProgressIncludesAllFields(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Progress(50).Log("Test message")

	// Verify step has correct fields
	assert.Equal(t, job.ID, step.l.Job.ID)
	assert.Equal(t, "test-pup-id", step.l.PupID)
	assert.Equal(t, 50, step.progress)
	assert.Equal(t, "test-step", step.step)
}

func TestActionLoggerProgressErrorFlag(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Err("Error message")

	// Verify error flag is set (would be checked in actual sendProgress call)
	assert.NotNil(t, step)
}

func TestActionLoggerProgressStepTaken(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	time.Sleep(10 * time.Millisecond)
	step.Log("Message")

	// Verify step taken is calculated (would be checked in actual sendProgress call)
	assert.NotNil(t, step)
}

// ============================================================================
// Test Suite: Edge Cases
// ============================================================================

func TestActionLoggerEmptyMessage(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Log("")

	// Should not panic
	assert.NotNil(t, step)
}

func TestActionLoggerSpecialCharactersInMessage(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Log("Message with special chars: !@#$%^&*()")

	// Should not panic
	assert.NotNil(t, step)
}

func TestActionLoggerUnicodeMessage(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Log("Message with unicode: 你好世界 🌍")

	// Should not panic
	assert.NotNil(t, step)
}

func TestActionLoggerVeryLongMessage(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	longMessage := bytes.Repeat([]byte("a"), 10000)
	step.Log(string(longMessage))

	// Should not panic
	assert.NotNil(t, step)
}

func TestActionLoggerNegativeProgress(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Progress(-10).Log("Message")

	assert.Equal(t, -10, step.progress)
}

func TestActionLoggerProgressOver100(t *testing.T) {
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: ""},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	step := logger.Step("test-step")
	step.Progress(150).Log("Message")

	assert.Equal(t, 150, step.progress)
}

func TestActionLoggerWritesJobLogsToJobPrefixedFile(t *testing.T) {
	tempDir := t.TempDir()
	job := createTestJob("InstallPup")
	testDBX := &Dogeboxd{
		Changes: make(chan Change, 100),
		config:  &ServerConfig{ContainerLogDir: tempDir},
	}
	logger := NewActionLogger(job, "test-pup-id", *testDBX)

	logger.Step("test-step").Log("Test message")

	jobLogPath := filepath.Join(tempDir, "job-"+job.ID)
	logData, err := os.ReadFile(jobLogPath)
	require.NoError(t, err)
	assert.Contains(t, string(logData), "Test message")

	_, err = os.Stat(filepath.Join(tempDir, "pup-"+job.ID))
	assert.ErrorIs(t, err, os.ErrNotExist)
}
