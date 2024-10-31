package main

import (
	"errors"
	"fmt"
	"os"
)

func setupWatchDataIfMissing() error {
	_, err := os.Stat(fileLocation)
	if err == nil {
		// if the file already exists, we don't need to do anything
		return nil
	}

	// if the file doesn't exist, we need to create it with the default headers
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check if data file exists: %w", err)
	}

	file, err := os.Create(fileLocation)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString("date,service,title,watch_time\n")
	if err != nil {
		return fmt.Errorf("failed to write headers to file: %w", err)
	}
	return nil
}
