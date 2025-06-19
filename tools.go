package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/hypermodeinc/modus/sdk/go/pkg/console"
	"github.com/hypermodeinc/modus/sdk/go/pkg/http"
	"github.com/hypermodeinc/modus/sdk/go/pkg/models/openai"
)

type toolsConfig struct {
	Tools []tool `json:"tools"`
}

type tool struct {
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Name        string          `json:"name"`
}

type AgentToolConnection struct {
	Name         string
	Description  string
	Type         string
	Parameters   string
	ProviderName string
	Endpoint     string
}

type Func func(args map[string]any) (string, error)

func getTools(agentToolConns []*AgentToolConnection) ([]*AgentToolConnection, error) {
	dirEntries, err := os.ReadDir("tools")
	if err != nil {
		console.Error("Failed to read tools directory: " + err.Error())
		return nil, err
	}

	for _, entry := range dirEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		filePath := "tools/" + entry.Name()
		providerName := ""
		name := entry.Name()
		if strings.HasSuffix(name, "_tools.json") {
			providerName = strings.TrimSuffix(name, "_tools.json")
		} else {
			providerName = strings.TrimSuffix(name, ".json")
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			console.Error("Failed to read " + filePath + ": " + err.Error())
			continue
		}

		var toolsConfig toolsConfig
		if err := json.Unmarshal(data, &toolsConfig); err != nil {
			console.Error("Failed to parse " + filePath + ": " + err.Error())
			continue
		}

		for _, t := range toolsConfig.Tools {
			agentToolConns = append(agentToolConns, &AgentToolConnection{
				Name:         t.Name,
				Description:  t.Description,
				Parameters:   string(t.InputSchema),
				ProviderName: providerName,
			})
		}
	}
	return agentToolConns, nil
}

func prefixToolName(provider, toolName string) string {
	return provider + "-" + toolName
}

func extractTools() (map[string]Func, error) {
	functions := make(map[string]Func)
	modelTools := make([]openai.Tool, 0)
	agentToolConns := make([]*AgentToolConnection, 0)

	agentToolConns, err := getTools(agentToolConns)
	if err != nil {
		return nil, err
	}

	for _, t := range agentToolConns {
		originalName := t.Name

		nameForModel := prefixToolName(t.ProviderName, t.Name)
		console.Infof("Adding tool: %s", nameForModel)
		modelTools = append(modelTools, openai.NewToolForFunction(nameForModel, t.Description).WithParametersSchema(t.Parameters))
		fn := func(args map[string]any) (string, error) {
			body := map[string]any{
				"tool":      originalName,
				"arguments": args,
			}

			if t.Type != "public" {
				if err := injectAuth(t.ProviderName, body); err != nil {
					return "", err
				}
			}

			bodyb, err := json.Marshal(body)
			if err != nil {
				return "", err
			}

			req := http.NewRequest(t.Endpoint, &http.RequestOptions{
				Method: "POST",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: bodyb,
			})

			resp, err := http.Fetch(req)
			if err != nil {
				return "", err
			}
			return resp.Text(), nil
		}
		functions[nameForModel] = fn
	}

	return functions, nil
}

func injectAuth(providerName string, body map[string]any) error {
	secretKey := "con_" + strings.Replace(providerName, "-", "_", -1)

	connToken := os.Getenv(secretKey)
	if connToken == "" {
		return fmt.Errorf("connection token not found, please authenticate your connection")
	}

	var mp map[string]any
	if err := json.Unmarshal([]byte(connToken), &mp); err != nil {
		return fmt.Errorf("failed to parse connection token: %w", err)
	}

	accessToken, err := getAndAssertString(mp, "accessToken")
	if err != nil {
		return fmt.Errorf("%w for provider %s", err, providerName)
	}
	body["token"] = accessToken

	return nil
}

func getAndAssertString(mp map[string]any, key string) (string, error) {
	val, ok := mp[key]
	if !ok {
		return "", fmt.Errorf("missing %s in connection token", key)
	}
	str, ok := val.(string)
	if !ok || str == "" {
		return "", fmt.Errorf("invalid %s type in connection token, expected string", key)
	}
	return str, nil
}
