package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hypermodeinc/modus/sdk/go/pkg/console"
	"github.com/hypermodeinc/modus/sdk/go/pkg/models"
	"github.com/hypermodeinc/modus/sdk/go/pkg/models/openai"
)

type MessageStatus string

const (
	messageStatusPending    MessageStatus = "pending"
	messageStatusThinking   MessageStatus = "thinking"
	messageStatusGenerating MessageStatus = "generating"
	messageStatusDone       MessageStatus = "done"
	messageStatusError      MessageStatus = "error"
)

type message struct {
	Id          []uint8         `json:"id"`
	AgentId     []uint8         `json:"agent_id"`
	Content     string          `json:"content"`
	Role        string          `json:"role"`
	Raw         json.RawMessage `json:"raw"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	AgentAvatar string          `json:"agent_avatar"`
	AgentName   string          `json:"agent_name"`
}

type Message struct {
	Id        string
	AgentId   string
	Content   string
	Raw       string
	Status    string
	Role      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (m message) convert() Message {
	rawStr := string(m.Raw)

	id, err := uuid.FromBytes(m.Id)
	if err != nil {
		console.Error(err.Error())
	}

	agentId := ""
	if len(m.AgentId) > 0 {
		a, err := uuid.FromBytes(m.AgentId)
		if err != nil {
			console.Error(err.Error())
		}
		agentId = a.String()
	}

	return Message{
		Id:        id.String(),
		AgentId:   agentId,
		Content:   m.Content,
		Raw:       rawStr,
		Status:    m.Status,
		Role:      m.Role,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func chat(message string, modelName string, systemPrompt string, tools map[string]Func) (*string, error) {

	model, err := models.GetModel[openai.ChatModel](modelName)
	if err != nil {
		return nil, err
	}

	initMessages, err := buildOpenAIMessages([]Message{}, systemPrompt)
	if err != nil {
		return nil, err
	}

	input, _ := model.CreateInput(initMessages...)
	input.ResponseFormat = openai.ResponseFormatText

	response, err := startAgentLoop(message, model, tools, input)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func buildOpenAIMessages(messages []Message, systemPrompt string) ([]openai.RequestMessage, error) {
	var result []openai.RequestMessage
	if systemPrompt != "" {
		result = append(result, openai.NewSystemMessage(systemPrompt))
	}
	for _, m := range messages {
		if m.Status == string(messageStatusPending) ||
			m.Status == string(messageStatusGenerating) ||
			m.Status == string(messageStatusThinking) {
			continue
		}
		switch m.Role {
		case "system":
			continue
		case "user":
			result = append(result, openai.NewUserMessage(m.Content))
		case "assistant":
			var mp map[string]any
			if err := json.Unmarshal([]byte(m.Raw), &mp); err != nil {
				return nil, err
			}
			astMsg := openai.NewAssistantMessage(m.Content)
			if _, ok := mp["tool_calls"]; ok {
				var toolCalls []openai.ToolCall
				jsonb, err := json.Marshal(mp["tool_calls"])
				if err != nil {
					return nil, err
				}
				if err := json.Unmarshal(jsonb, &toolCalls); err != nil {
					return nil, err
				}
				astMsg.ToolCalls = toolCalls
			}
			result = append(result, astMsg)
		case "tool":
			var mp map[string]any
			if err := json.Unmarshal([]byte(m.Raw), &mp); err != nil {
				return nil, err
			}
			toolCallId, _ := mp["tool_call_id"].(string)
			toolMsg := openai.NewToolMessage(m.Content, toolCallId)
			result = append(result, toolMsg)
		}
	}
	return result, nil
}

func startAgentLoop(initialAstMsgId string, model *openai.ChatModel, funcMap map[string]Func, input *openai.ChatModelInput) (string, error) {
	maxRetries := 5
	retryCount := 0

	nextAstMsgId := initialAstMsgId
	for {
		output, err := model.Invoke(input)
		if err != nil {
			shouldRetry := false
			errMsg := err.Error()

			switch {
			case strings.Contains(errMsg, "500 Internal Server Error"),
				strings.Contains(errMsg, "502 Bad Gateway"),
				strings.Contains(errMsg, "503 Service Unavailable"),
				strings.Contains(errMsg, "504 Gateway Timeout"):
				shouldRetry = true
			case strings.Contains(errMsg, "rate limit exceeded"), strings.Contains(errMsg, "too many requests"):
				shouldRetry = true
				console.Warn("Rate limit exceeded, retrying with backoff...")
			}

			if !shouldRetry {
				return nextAstMsgId, err
			}

			if retryCount >= maxRetries {
				return nextAstMsgId, fmt.Errorf("max retries exceeded: %w", err)
			}

			backoffDuration := time.Duration(1<<retryCount)*time.Second + time.Duration(rand.Intn(1000))*time.Millisecond
			retryCount++
			time.Sleep(backoffDuration)
			continue
		}

		retryCount = 0
		msg := output.Choices[0].Message
		console.Debug("Response content: " + msg.Content)

		if len(msg.ToolCalls) > 0 {
			for i := range msg.ToolCalls {
				console.Debug("Tool call: " + msg.ToolCalls[i].Function.Name)
			}
			astMsg := msg.ToAssistantMessage()
			input.Messages = append(input.Messages, astMsg)

			for _, tc := range msg.ToolCalls {
				var toolMsg *openai.ToolMessage[string]
				if tc.Function.Name == "github_auth_request" {
					fmt.Println("github_auth_request")
					return "", nil
				} else if fn, ok := funcMap[tc.Function.Name]; ok {

					var args map[string]any
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						toolMsg = openai.NewToolMessage("Error constructing tool arguments: "+err.Error(), tc.Id)
						input.Messages = append(input.Messages, toolMsg)
						continue
					}

					res, err := fn(args)
					if err != nil {
						toolMsg = openai.NewToolMessage(fmt.Sprintf("Error invoking tool: %s", err.Error()), tc.Id)
					} else {
						toolMsg = openai.NewToolMessage(res, tc.Id)
					}
				} else {
					console.Error("Unknown tool called: " + tc.Function.Name)
					toolMsg = openai.NewToolMessage("Unknown tool called: "+tc.Function.Name, tc.Id)
				}

				input.Messages = append(input.Messages, toolMsg)
			}
		} else if msg.Content != "" {
			return "", nil
		} else {
			return nextAstMsgId, errors.New("invalid response from model")
		}
	}
}
