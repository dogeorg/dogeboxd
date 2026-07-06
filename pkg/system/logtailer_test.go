package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogTailerGetPagePaginatesOlderLines(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "pup-test-pup")

	lines := make([]string, 2505)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%d", i+1)
	}

	err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	require.NoError(t, err)

	tailer := NewLogTailer()

	firstPage, err := tailer.GetPage(logPath, nil, 1000)
	require.NoError(t, err)
	require.Len(t, firstPage.Lines, 1000)
	require.NotNil(t, firstPage.ResumeToken)
	require.NotNil(t, firstPage.OlderCursor)
	assert.True(t, firstPage.HasMoreOlder)
	assert.Equal(t, "line-1506", firstPage.Lines[0])
	assert.Equal(t, "line-2505", firstPage.Lines[len(firstPage.Lines)-1])

	secondPage, err := tailer.GetPage(logPath, parseOffset(t, firstPage.OlderCursor), 1000)
	require.NoError(t, err)
	require.Len(t, secondPage.Lines, 1000)
	require.Nil(t, secondPage.ResumeToken)
	require.NotNil(t, secondPage.OlderCursor)
	assert.True(t, secondPage.HasMoreOlder)
	assert.Equal(t, "line-506", secondPage.Lines[0])
	assert.Equal(t, "line-1505", secondPage.Lines[len(secondPage.Lines)-1])

	finalPage, err := tailer.GetPage(logPath, parseOffset(t, secondPage.OlderCursor), 1000)
	require.NoError(t, err)
	require.Len(t, finalPage.Lines, 505)
	require.Nil(t, finalPage.ResumeToken)
	require.Nil(t, finalPage.OlderCursor)
	assert.False(t, finalPage.HasMoreOlder)
	assert.Equal(t, "line-1", finalPage.Lines[0])
	assert.Equal(t, "line-505", finalPage.Lines[len(finalPage.Lines)-1])
}

func TestLogTailerGetPageHandlesBeforeStartOfFile(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "pup-test-pup")

	err := os.WriteFile(logPath, []byte("line-1\nline-2\n"), 0o644)
	require.NoError(t, err)

	tailer := NewLogTailer()

	beforeStart := int64(0)
	page, err := tailer.GetPage(logPath, &beforeStart, 1000)
	require.NoError(t, err)
	assert.Empty(t, page.Lines)
	assert.Nil(t, page.ResumeToken)
	assert.Nil(t, page.OlderCursor)
	assert.False(t, page.HasMoreOlder)
}

func parseOffset(t *testing.T, offset *string) *int64 {
	t.Helper()
	require.NotNil(t, offset)

	parsed, err := strconv.ParseInt(*offset, 10, 64)
	require.NoError(t, err)

	return &parsed
}
