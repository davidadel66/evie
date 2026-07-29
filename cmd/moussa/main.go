// Command agent is the moussa agent harness — the nucleus of this repo.
// It runs an interactive REPL: user input goes to an LLM via OpenRouter,
// and any tool calls the model makes are executed against the registry in
// tools.go, with results fed back until the model answers in plain text.
//
// Every capability under internal/ (todo, finance, ...) is meant to be
// exposed to the model here as an AgentTool over time.
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/davidadel66/moussa/internal/openrouter"
	"github.com/davidadel66/moussa/internal/tools"

	"github.com/joho/godotenv"
)

// main runs the chat loop. Outer loop: one user turn. Inner loop: call the
// model, execute every tool call it requests, and go around again until the
// model responds with no tool calls — that response ends the turn. Request
// failures print and return to the prompt rather than killing the session.
func main() {
	_ = godotenv.Load("../../.env")
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := "moonshotai/kimi-k3"
	client, err := openrouter.NewClient(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	messages := []openrouter.Message{
		{
			Role:    "system",
			Content: "You're name is Eve - A helpful assistant to David with the intent to be his personal 'jarvis'",
		},
	}

	scanner := bufio.NewScanner(os.Stdin)

	// approve is the terminal half of the write gate: gated tools show
	// what they're about to run and wait for a y/yes before executing.
	// It shares the REPL's scanner — stdin has exactly one reader.
	approve := func(name, args string) bool {
		fmt.Printf("\n[%s wants to run]\n%s\napprove? [y/N] ", name, args)
		if !scanner.Scan() {
			return false
		}
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return answer == "y" || answer == "yes"
	}

	for {
		fmt.Print("< ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		userMsg := openrouter.Message{
			Role:    "user",
			Content: input,
		}

		messages = append(messages, userMsg)

		for {
			req := openrouter.ChatRequest{
				Model:    model,
				Messages: messages,
				Tools:    tools.Schemas(),
			}

			res, err := client.Chat(req)
			if err != nil {
				fmt.Printf("request failed: %v\n", err)
				break
			}

			if len(res.Choices) == 0 {
				fmt.Println("no response returned")
				break
			}

			messages = append(messages, res.Choices[0].Message)
			fmt.Println(res.Choices[0].Message.Content)

			if len(res.Choices[0].Message.ToolCalls) == 0 {
				break
			}

			for _, toolCall := range res.Choices[0].Message.ToolCalls {
				fmt.Printf("[calling %s]\n", toolCall.Function.Name)
				messages = append(messages, tools.Execute(toolCall, approve))
			}

		}

	}
}
