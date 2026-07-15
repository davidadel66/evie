package main

import (
	"bufio"
	"fmt"
	"log"
	"os"

	"github.com/davidadel66/moussa/internal/openrouter"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load("../../.env")
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := "deepseek/deepseek-v4-flash"
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
				Tools:    ToolSchemas(),
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
				messages = append(messages, ExecuteTool(toolCall))
			}

		}

	}
}
