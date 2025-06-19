package main
  
import (
  "encoding/json"

  "github.com/hypermodeinc/modus/sdk/go/pkg/agents"
)

type Agent struct {
  agents.AgentBase
  
  Description string
  SystemPrompt string
  Model string
  Temperature float64
  Tools map[string]Func
}

func (a *Agent) Name() string {
  return "Bob"
}

func (a *Agent) GetState() *string {
  state := struct {
    Description   string                  `json:"description"`
    SystemPrompt  string                  `json:"systemprompt"`
    Model         string                  `json:"model"`
    Temperature   float64                 `json:"temperature"`
    Tools         map[string]Func         `json:"tools"`
  }{
    Description:  a.Description,
    SystemPrompt: a.SystemPrompt,
    Model:        a.Model,
    Temperature:  a.Temperature,
    Tools:        a.Tools,
  }
  if bytes, err := json.Marshal(state); err == nil {
    str := string(bytes)
    return &str
  }
  return nil
}

func (a *Agent) SetState(data *string) {
  if data == nil || *data == "" {
    return
  }
  state := struct {
    Description   string                 `json:"description"`
    SystemPrompt  string                 `json:"systemprompt"`
    Model         string                 `json:"model"`
    Tools         map[string]Func        `json:"tools"`
  }{}
  if err := json.Unmarshal([]byte(*data), &state); err == nil {
    a.Description = state.Description
    a.SystemPrompt = state.SystemPrompt
    a.Model = state.Model
    a.Tools = state.Tools
  }
}

func (a *Agent) OnStart() error {
  a.Description = "SpecSpark"
  a.SystemPrompt = `Identity

Name: SpecSpark

Purpose: Help hobbyists learn from their project discussions by fetching chat history files from GitHub, extracting topics, and generating flashcards.

Context

User: Hobbyist with a connected GitHub account
On start, list all available GitHub repositories for the user
Prompt the user to select a repository
Fetch all cursor-chat-history files from the /.specstory subfolder of the selected repo
Analyze the chat histories to extract main topics
For each topic, generate a set of flashcards (Q&A)
Present the topics and flashcards as a simple text list
The agent will decide the number, difficulty, and style of flashcards for each topic
2. GitHub Integration
In Hypermode, go to the Integrations section.
Connect your GitHub account (if you haven’t already).
Make sure the agent has permission to list your repositories and read file contents.
3. Input & Workflow
Set the agent’s first step to:
List all GitHub repositories available to the user.
Prompt the user to select one.
Fetch all files from the /.specstory subfolder of the selected repo.
Process the files as described in the system prompt.
4. Output
Configure the agent to output a simple text list of topics and their associated flashcards.
`
  a.Model = "gpt-4.1-mini"
  a.Temperature = 0.2
  
  tools, err := extractTools()
  if err != nil {
    return err
  }
  a.Tools = tools
  
  return nil
}

func (a *Agent) OnSuspend() error {
  return nil
}

func (a *Agent) OnRestore() error {
  return nil
}

func (a *Agent) OnTerminate() error {
  return nil
}

func (a *Agent) OnReceiveMessage(msgName string, data *string) (*string, error) {
  chat(msgName, a.Model, a.SystemPrompt, a.Tools)
  
  return nil, nil
}
