package system

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/sdjournal"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func NewJournalReader(config dogeboxd.ServerConfig) JournalReader {
	return JournalReader{
		config: config,
	}
}

type JournalReader struct {
	config dogeboxd.ServerConfig
}

/*
	func (t JournalReader) GetJournalChan(service string) (context.CancelFunc, chan string, error) {
		ctx, cancel := context.WithCancel(context.Background())

		out := make(chan string, 10)

		go func() {
			j, err := sdjournal.NewJournal()
			if err != nil {
				fmt.Println(err)
				return
			}
			defer j.Close()

			// Add a match for the specific service
			err = j.AddMatch(fmt.Sprintf("_SYSTEMD_UNIT=%s", service))
			if err != nil {
				fmt.Println(err)
				return
			}

			// Seek to the end of the journal
			err = j.SeekTail()
			if err != nil {
				fmt.Println(err)
				return
			}

			// skip back 50 lines..
			_, err = j.PreviousSkip(50)
			if err != nil {
				fmt.Println(err)
				return
			}

			for {
				select {
				case <-ctx.Done():
					break
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
*/
func (t JournalReader) GetJournalChan(machineID, service string) (context.CancelFunc, chan string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan string, 10)

	j, err := sdjournal.NewJournal()
	if err != nil {
		return cancel, nil, err
	}
	defer j.Close()

	// Add matches for both the machine ID and the service unit
	if err := j.AddMatch(fmt.Sprintf("_MACHINE_ID=%s", machineID)); err != nil {
		return cancel, nil, err
	}
	if err := j.AddMatch(fmt.Sprintf("_SYSTEMD_UNIT=%s", service)); err != nil {
		return cancel, nil, err
	}

	// Seek to the end of the journal
	if err := j.SeekTail(); err != nil {
		return cancel, nil, err
	}

	// Skip back 50 lines
	if _, err := j.PreviousSkip(50); err != nil {
		return cancel, nil, err
	}

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				i, err := j.Next()
				if err != nil {
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
