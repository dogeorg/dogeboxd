package system

import (
	"errors"
	"testing"

	"github.com/coreos/go-systemd/sdjournal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeJournalNavigator struct {
	entries []journalEntry
	index   int
	err     error
}

func (f *fakeJournalNavigator) Previous() (uint64, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.index >= len(f.entries) {
		return 0, nil
	}

	f.index++
	return 1, nil
}

func (f *fakeJournalNavigator) GetEntry() (*sdjournal.JournalEntry, error) {
	entry := f.entries[f.index-1]
	return &sdjournal.JournalEntry{
		Fields: map[string]string{"MESSAGE": entry.message},
	}, nil
}

func (f *fakeJournalNavigator) GetCursor() (string, error) {
	return f.entries[f.index-1].cursor, nil
}

func TestCollectJournalEntriesBackwardReturnsChronologicalPage(t *testing.T) {
	navigator := &fakeJournalNavigator{
		entries: []journalEntry{
			{message: "line-5", cursor: "cursor-5"},
			{message: "line-4", cursor: "cursor-4"},
			{message: "line-3", cursor: "cursor-3"},
		},
	}

	entries, hasMoreOlder, err := collectJournalEntriesBackward(navigator, 2)
	require.NoError(t, err)

	assert.True(t, hasMoreOlder)
	assert.Equal(t, []journalEntry{
		{message: "line-4", cursor: "cursor-4"},
		{message: "line-5", cursor: "cursor-5"},
	}, entries)
}

func TestCollectJournalEntriesBackwardPropagatesErrors(t *testing.T) {
	navigator := &fakeJournalNavigator{err: errors.New("boom")}

	entries, hasMoreOlder, err := collectJournalEntriesBackward(navigator, 2)
	require.Error(t, err)
	assert.Nil(t, entries)
	assert.False(t, hasMoreOlder)
}
