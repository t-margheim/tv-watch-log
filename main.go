package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
)

const fileLocation = "./watching_data.csv"

//go:embed prompt.txt
var prompt string

var exitCommands = map[string]bool{
	"exit": true,
	"quit": true,
	"q":    true,
	"bye":  true,
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelInfo)
	err := setupWatchDataIfMissing()
	if err != nil {
		slog.Error("Error setting up watch data", "error", err)
		os.Exit(1)
	}

	err = godotenv.Load()
	if err != nil {
		slog.Error("Error loading .env file", "error", err)
		os.Exit(1)
	}
	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	tvShowInfoTool := setupShowInfoTool()

	tools := []openai.Tool{tvShowInfoTool}
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: prompt,
			},
		},
		Tools: tools,
	}

	runConversation(client, req, tools)
}

func runConversation(client *openai.Client, req openai.ChatCompletionRequest, tools []openai.Tool) {
	ctx := context.Background()
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
			slog.Info("Tool called", "function_name", toolCall.Function.Name)
			if toolCall.Function.Name == "get_show_info" {
				rawArgs := toolCall.Function.Arguments
				slog.Debug("Tool call arguments", "args", rawArgs)
				var args toolArgs
				err := json.Unmarshal([]byte(rawArgs), &args)
				if err != nil {
					slog.Error("Error unmarshalling tool call arguments", "error", err)
					continue
				}

				slog.Info("Tool call arguments", "args", args)
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
					Tools:    tools,
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
			)
			fmt.Println(responseText)
		} else {
			fmt.Println("Updated file with new data")
		}

		req.Messages = append(req.Messages, resp.Choices[0].Message)
		fmt.Print("> ")
	}
}
