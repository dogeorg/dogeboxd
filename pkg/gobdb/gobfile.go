package gobdb

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
)

/* GobFile is a simple atomic-write single-file-database
 * which stores a Go object encoded with encoding/gob.
 *
 * Usage:
 *  gf := &GobFile[YourTypeHere]{filename: "yourDB.gob"}
 *  err := gf.Save(YourObj)
 *  obj, err := gf.Load()
 */
type GobFile[T any] struct {
	filename string
}

func (gf *GobFile[T]) Save(obj T) error {
	tempFile, err := os.CreateTemp("", "temp_gob_file")
	if err != nil {
		return fmt.Errorf("cannot create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	encoder := gob.NewEncoder(tempFile)
	if err := encoder.Encode(obj); err != nil {
		return fmt.Errorf("cannot encode object: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("cannot close temporary file: %w", err)
	}

	if err := os.Rename(tempFile.Name(), gf.filename); err != nil {
		return fmt.Errorf("cannot rename temporary file to %q: %w", gf.filename, err)
	}

	return nil
}

func (gf *GobFile[T]) Load() (T, error) {
	file, err := os.Open(gf.filename)
	if err != nil {
		return *new(T), fmt.Errorf("cannot open file %q: %w", gf.filename, err)
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	var obj T
	if err := decoder.Decode(&obj); err != nil {
		if err == io.EOF {
			return *new(T), fmt.Errorf("file %q is empty", gf.filename)
		}
		return *new(T), fmt.Errorf("cannot decode object from file %q: %w", gf.filename, err)
	}

	return obj, nil
}
