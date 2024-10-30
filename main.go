package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

const fileLocation = "/Users/timothymargheim/repos/tv-watch-log/watching_data.csv"

//go:embed prompt.txt
var prompt string

var exitCommands = map[string]bool{
	"exit": true,
	"quit": true,
	"q":    true,
	"bye":  true,
}

type tvdbResponse struct {
	Data []tvdbData `json:"data"`
}

type tvdbData struct {
	Country string
	Name    string
	Network string
}
type contentInfo struct {
	Title   string
	Service string
}

func getShowInfo(query string) contentInfo {
	slog.Info("Getting show info", "query", query)
	baseURL := "https://api4.thetvdb.com/v4/search"
	qParams := url.QueryEscape(query)

	reqURL := fmt.Sprintf("%s?query=%s", baseURL, qParams)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		slog.Error("Error creating request", "error", err)
		return contentInfo{}
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("TVDB_TOKEN")))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("Error making request", "error", err)
		return contentInfo{}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Error reading response body", "error", err)
		return contentInfo{}
	}
	slog.Debug("Response body", "body", string(respBody))

	var tvdbResp tvdbResponse
	err = json.Unmarshal(respBody, &tvdbResp)
	if err != nil {
		slog.Error("Error unmarshalling response body", "error", err)
		return contentInfo{}
	}

	slog.Info("Got TVDB response", "response", tvdbResp)

	var usaData tvdbData
	for _, data := range tvdbResp.Data {
		if strings.EqualFold("usa", data.Country) {
			usaData = data
			break
		}
	}
	ci := contentInfo{
		Title:   usaData.Name,
		Service: usaData.Network,
	}
	slog.Info("Got show info", "info", ci)
	return ci
}

type toolArgs struct {
	Query string `json:"query_string"`
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelInfo)
	ctx := context.Background()
	err := godotenv.Load()
	if err != nil {
		slog.Error("Error loading .env file", "error", err)
		os.Exit(1)
	}
	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	// describe the function & its inputs
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"query_string": {
				Type: jsonschema.String,
				Description: "Search string for the content," +
					" e.g. 'The Office', 'Bachelor', 'Agatha all along",
			},
		},
		Required: []string{"query_string"},
	}
	f := openai.FunctionDefinition{
		Name:        "get_show_info",
		Description: "Get name and service information for a specific show",
		Parameters:  params,
	}

	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: prompt,
			},
		},
		Tools: []openai.Tool{t},
	}
	fmt.Println("Conversation")
	fmt.Println("---------------------")
	fmt.Print("> ")
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		txt := s.Text()
		if exitCommands[txt] {
			break
		}
		req.Messages = append(req.Messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: txt,
		})
		resp, err := client.CreateChatCompletion(ctx, req)
		if err != nil {
			fmt.Printf("ChatCompletion error: %v\n", err)
			continue
		}

		msg := resp.Choices[0].Message
		req.Messages = append(req.Messages, msg)
		if len(msg.ToolCalls) > 0 {
			toolCall := msg.ToolCalls[0]
			slog.Info("Tool call", "toolCall", toolCall)
			if toolCall.Function.Name == "get_show_info" {
				rawArgs := toolCall.Function.Arguments
				slog.Info("Tool call arguments", "args", rawArgs)
				var args toolArgs
				err := json.Unmarshal([]byte(rawArgs), &args)
				if err != nil {
					slog.Error("Error unmarshalling tool call arguments", "error", err)
					continue
				}

				info := getShowInfo(args.Query)
				req.Messages = append(req.Messages, openai.ChatCompletionMessage{
					Role: openai.ChatMessageRoleTool,
					Content: fmt.Sprintf(
						"The show is %s and it is available on %s",
						info.Title, info.Service,
					),
					ToolCallID: toolCall.ID,
				})
			}
			resp, err = client.CreateChatCompletion(ctx,
				openai.ChatCompletionRequest{
					Model:    openai.GPT4TurboPreview,
					Messages: req.Messages,
					Tools:    []openai.Tool{t},
				},
			)
			if err != nil {
				slog.Error("Error creating chat completion", "error", err)
				continue
			}
		}

		slog.Debug("Response received", "response", resp)
		responseText := resp.Choices[0].Message.Content

		err = processMessage(responseText)
		if err != nil {
			slog.Error("Error processing response text",
				"error", err,
				"responseText", responseText,
			)
		}

		req.Messages = append(req.Messages, resp.Choices[0].Message)
		fmt.Print("> ")
	}
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
		slog.Info("date set", "date", row.date, "days_offset", row.DaysOffset)
		_, err := file.WriteString(fmt.Sprintf(
			"%s,%s,%s,%d\n",
			row.date, row.Service, row.Title, row.WatchTime,
		))
		if err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}

		slog.Info("Wrote row to file",
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

type viewData struct {
	DaysOffset int `json:"days_offset"`
	date       string
	Service    string `json:"service"`
	Title      string `json:"title"`
	WatchTime  int    `json:"watch_time"`
}
