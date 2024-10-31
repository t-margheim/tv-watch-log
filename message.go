package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

type viewData struct {
	DaysOffset int `json:"days_offset"`
	date       string
	Service    string `json:"service"`
	Title      string `json:"title"`
	WatchTime  int    `json:"watch_time"`
}

func processMessage(message string) error {
	slog.Info("Processing message", "message", message)

	if strings.Contains(message, "```") {
		var err error
		message, err = cleanMessage(message)
		if err != nil {
			return err
		}
	}

	var viewDataRows []viewData
	err := json.Unmarshal([]byte(message), &viewDataRows)
	if err != nil {
		return fmt.Errorf("failed to unmarshal: %w", err)
	}

	if len(viewDataRows) == 0 {
		slog.Warn("No view data rows found",
			"message", message)
		return nil
	}
	return writeToFile(viewDataRows)
}

func cleanMessage(message string) (string, error) {
	startIdx := strings.Index(message, "```")
	endIdx := strings.LastIndex(message, "```")
	if startIdx == -1 || endIdx == -1 {
		return message, nil
	}

	if endIdx == startIdx {
		return "", fmt.Errorf("could not parse message")
	}

	message = strings.TrimSpace(message[startIdx+3 : endIdx])
	message = strings.TrimPrefix(message, "json")

	return message, nil
}

func writeToFile(viewDataRows []viewData) error {
	file, err := os.OpenFile(fileLocation, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	for _, row := range viewDataRows {
		row.date = getDateFromOffset(row.DaysOffset)
		slog.Debug("date set", "date", row.date, "days_offset", row.DaysOffset)
		_, err := file.WriteString(fmt.Sprintf(
			"%s,%s,%s,%d\n",
			row.date, row.Service, row.Title, row.WatchTime,
		))
		if err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}

		slog.Info("Wrote row to file")
		slog.Debug("Wrote row to file",
			"date", row.date,
			"service", row.Service,
			"title", row.Title,
			"watch_time", row.WatchTime,
		)

	}
	return nil
}

func getDateFromOffset(offset int) string {
	return time.Now().AddDate(0, 0, offset).Format("2006-01-02")
}
