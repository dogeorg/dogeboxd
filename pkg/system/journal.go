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
