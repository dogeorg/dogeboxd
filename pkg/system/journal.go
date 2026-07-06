package system

import (
	"context"
	"fmt"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/coreos/go-systemd/sdjournal"
)

type journalEntry struct {
	message string
	cursor  string
}

func NewJournalReader(config dogeboxd.ServerConfig) JournalReader {
	return JournalReader{
		config: config,
	}
}

type JournalReader struct {
	config dogeboxd.ServerConfig
}

func (t JournalReader) GetJournalChannel(service string) (context.CancelFunc, chan string, error) {
	return t.getJournalChannel(service, nil)
}

func (t JournalReader) GetJournalChannelFromCursor(service string, cursor string) (context.CancelFunc, chan string, error) {
	return t.getJournalChannel(service, &cursor)
}

func (t JournalReader) getJournalChannel(service string, cursor *string) (context.CancelFunc, chan string, error) {
	ctx, cancel := context.WithCancel(context.Background())

	out := make(chan string, 10)

	go func() {
		j, err := sdjournal.NewJournal()
		if err != nil {
			fmt.Println(err)
			close(out)
			return
		}
		defer j.Close()

		// Add a match for the specific service
		err = j.AddMatch(fmt.Sprintf("_SYSTEMD_UNIT=%s", service))
		if err != nil {
			fmt.Println(err)
			close(out)
			return
		}

		if cursor != nil {
			err = j.SeekCursor(*cursor)
			if err != nil {
				fmt.Println(err)
				close(out)
				return
			}

			// Advance once so subsequent reads only emit entries after the cursor.
			_, err = j.Next()
			if err != nil {
				fmt.Println(err)
				close(out)
				return
			}
		} else {
			// Seek to the end of the journal
			err = j.SeekTail()
			if err != nil {
				fmt.Println(err)
				close(out)
				return
			}
		}

		for {
			select {
			case <-ctx.Done():
				close(out)
				return
			default:
				i, err := j.Next()
				if err != nil {
					fmt.Println("!!", err)
					continue
				}

				if i == 0 {
					time.Sleep(time.Second)
					continue
				}

				entry, err := j.GetEntry()
				if err != nil {
					continue
				}

				out <- entry.Fields["MESSAGE"]
			}
		}
	}()
	return cancel, out, nil
}

func (t JournalReader) GetJournalTail(service string, limit int) ([]string, *string, error) {
	page, err := t.GetJournalPage(service, nil, limit)
	if err != nil {
		return nil, nil, err
	}

	return page.Lines, page.ResumeToken, nil
}

func (t JournalReader) GetJournalPage(service string, before *string, limit int) (dogeboxd.LogPage, error) {
	if limit <= 0 {
		return dogeboxd.LogPage{}, fmt.Errorf("Log tail limit must be greater than zero")
	}

	j, err := sdjournal.NewJournal()
	if err != nil {
		return dogeboxd.LogPage{}, err
	}
	defer j.Close()

	err = j.AddMatch(fmt.Sprintf("_SYSTEMD_UNIT=%s", service))
	if err != nil {
		return dogeboxd.LogPage{}, err
	}

	if before != nil {
		err = j.SeekCursor(*before)
	} else {
		err = j.SeekTail()
	}
	if err != nil {
		return dogeboxd.LogPage{}, err
	}

	entries, hasMoreOlder, err := collectJournalEntriesBackward(j, limit)
	if err != nil {
		return dogeboxd.LogPage{}, err
	}

	page := dogeboxd.LogPage{
		Lines:        make([]string, len(entries)),
		HasMoreOlder: hasMoreOlder,
	}
	for i, entry := range entries {
		page.Lines[i] = entry.message
	}
	if len(entries) > 0 {
		resumeToken := entries[len(entries)-1].cursor
		page.ResumeToken = &resumeToken
	}
	if hasMoreOlder && len(entries) > 0 {
		olderCursor := entries[0].cursor
		page.OlderCursor = &olderCursor
	}

	return page, nil
}

type journalNavigator interface {
	Previous() (uint64, error)
	GetEntry() (*sdjournal.JournalEntry, error)
	GetCursor() (string, error)
}

func collectJournalEntriesBackward(j journalNavigator, limit int) ([]journalEntry, bool, error) {
	entriesNewestFirst := make([]journalEntry, 0, limit+1)
	for len(entriesNewestFirst) < limit+1 {
		n, err := j.Previous()
		if err != nil {
			return nil, false, err
		}
		if n == 0 {
			break
		}

		entry, err := j.GetEntry()
		if err != nil {
			return nil, false, err
		}

		cursor, err := j.GetCursor()
		if err != nil {
			return nil, false, err
		}

		entriesNewestFirst = append(entriesNewestFirst, journalEntry{
			message: entry.Fields["MESSAGE"],
			cursor:  cursor,
		})
	}

	hasMoreOlder := len(entriesNewestFirst) > limit
	if hasMoreOlder {
		entriesNewestFirst = entriesNewestFirst[:limit]
	}

	entries := make([]journalEntry, len(entriesNewestFirst))
	for i := range entriesNewestFirst {
		entries[i] = entriesNewestFirst[len(entriesNewestFirst)-1-i]
	}

	return entries, hasMoreOlder, nil
}
