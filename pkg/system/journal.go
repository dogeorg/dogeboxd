package system

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/sdjournal"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func NewJournalReader(config dogeboxd.ServerConfig, match string) JournalReader {
	return JournalReader{
		config: config,
		match:  match,
		out:    make(chan string, 10),
	}
}

type JournalReader struct {
	config dogeboxd.ServerConfig
	match  string
	out    chan string
}

func (t JournalReader) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		go func() {
			j, err := sdjournal.NewJournal()
			if err != nil {
				fmt.Println(err)
				return
			}
			defer j.Close()

			// Add a match for the specific service

			err = j.AddMatch("_SYSTEMD_UNIT=systemd-logind.service")
			if err != nil {
				fmt.Println(err)
				return
			}

			// Seek to the end of the journal
			err = j.SeekHead()
			if err != nil {
				fmt.Println(err)
				return
			}

			for {
				var entry *sdjournal.JournalEntry

				i, err := j.Next()
				if err != nil {
					fmt.Println("!!", err)
					continue
				}

				if i == 0 {
					time.Sleep(time.Second)
					continue
				}

				entry, err = j.GetEntry()
				if err != nil {
					continue
				}

				fmt.Printf("log> %s\n", entry.Fields["MESSAGE"])
			}
		}()
		/*
			go func() {
			mainloop:
				for {
					select {
					case <-stop:
						break mainloop
					}
				}
			}()
		*/

		started <- true
		<-stop
		// do shutdown things
		stopped <- true
	}()
	return nil
}
