package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

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

	slog.Info("Queried TVDB", "results", len(tvdbResp.Data))

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

func setupShowInfoTool() openai.Tool {
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
	return t
}
