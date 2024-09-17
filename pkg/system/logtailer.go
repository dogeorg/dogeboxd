package system

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func NewLogTailer(config dogeboxd.ServerConfig) LogTailer {
	return LogTailer{
		config: config,
	}
}

type LogTailer struct {
	config dogeboxd.ServerConfig
}

var dir = "/var/log/containers"

func (t LogTailer) GetChan(pupId string) (context.CancelFunc, chan string, error) {
	ctx, cancel := context.WithCancel(context.Background())

	out := make(chan string, 10)

	go func() {
		file, err := os.Open(filepath.Join(dir, "pup-"+pupId))
		if err != nil {
			close(out)
			log.Printf("Error opening log file: %+v", err)
			return
		}
		defer file.Close()

		log.Printf("Opened log file: %s", file.Name())

		// Seek to the end of the file
		_, err = file.Seek(0, io.SeekEnd)
		if err != nil {
			close(out)
			return
		}

		reader := bufio.NewReader(file)

		for {
			select {
			case <-ctx.Done():
				close(out)
				return
			default:
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						time.Sleep(100 * time.Millisecond)
						continue
					}
					close(out)
					return
				}
				out <- line
			}
		}

	}()
	return cancel, out, nil
}
