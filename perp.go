package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/pflag"
)

// Message represents a message in the conversation.
type Message struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// RequestPayload is the structure sent to the Perplexity API.
type RequestPayload struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// StreamingChoice represents one choice in the streaming response.
type StreamingChoice struct {
	Delta   Message `json:"delta"`
	Message Message `json:"message"`
}

// StreamingResponse represents a single chunk from the API stream.
type StreamingResponse struct {
	Choices   []StreamingChoice `json:"choices"`
	Citations []string          `json:"citations,omitempty"`
}

func main() {
	// Parse command-line flags.
	var model string
	var cite bool
	pflag.StringVarP(&model, "model", "m", "sonar", "Model name to use (defaults to mistral-large-latest)")
	pflag.BoolVarP(&cite, "cite", "c", false, "citing mode (display citations)")
	pflag.Parse()

	// Ensure the query is provided.
	args := pflag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: perp \"<query>\" [-m <model name>] [-c]")
		os.Exit(1)
	}
	query := args[0]

	// Build the request payload.
	payload := RequestPayload{
		Model:  model,
		Stream: true, // Enable streaming.
		Messages: []Message{
			{Role: "system", Content: "Be precise and concise."},
			{Role: "user", Content: query},
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error marshaling payload:", err)
		os.Exit(1)
	}

	// Get the API token from the environment.
	apiKey := os.Getenv("PERPLEXITY_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set your PERPLEXITY_API_KEY environment variable")
		os.Exit(1)
	}

	// Create and send the HTTP request.
	req, err := http.NewRequest("POST", "https://api.perplexity.ai/chat/completions", bytes.NewReader(jsonPayload))
	if err != nil {
		fmt.Println("Error creating request:", err)
		os.Exit(1)
	}
	req.Header.Add("Authorization", "Bearer "+apiKey)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: received status %d\n%s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}
	var citations []string

	// Process the stream.
	reader := bufio.NewReader(resp.Body)
	for {
		// Read one line at a time.
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Println("Error reading stream:", err)
			break
		}

		// Trim whitespace and skip if empty.
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Handle streaming format with "data:" prefix.
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			// Check for the end of the stream.
			if data == "[DONE]" {
				break
			}

			// Parse the JSON chunk.
			var streamResp StreamingResponse
			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
				fmt.Println("Error parsing JSON:", err)
				continue
			}
			if len(streamResp.Choices) > 0 && len(citations) == 0 {
				citations = streamResp.Citations
			}

			// Print only the text content.
			for _, choice := range streamResp.Choices {
				// Prefer the delta content (incremental update), but if empty, use full message content.
				content := choice.Delta.Content
				if content == "" {
					content = choice.Message.Content
				}
				fmt.Print(content)
			}
		} else {
			// In case the API returns a JSON object directly.
			var streamResp StreamingResponse
			if err := json.Unmarshal([]byte(line), &streamResp); err != nil {
				continue
			}
			for _, choice := range streamResp.Choices {
				content := choice.Delta.Content
				if content == "" {
					content = choice.Message.Content
				}
				fmt.Print(content)
			}

		}
	}
	// Print citations as clickable links.
	if len(citations) != 0 && cite {

		fmt.Println("\n\nCitations:")
		for i, citation := range citations {
			fmt.Printf("[%d]: %s\t", i+1, citation)
		}
	}

}
