package main

import (
	"github.com/hypermodeinc/modus/sdk/go/pkg/agents"
)

func init() {
	agents.Register(&Agent{})
}

func StartAgent() (agents.AgentInfo, error) {
	return agents.Start("Bob")
}

func MessageAgent(message string) (string, error) {
	resp, err := agents.SendMessage("Bob", message)
	if err != nil {
		return "", err
	}
	return *resp, nil
}

func TerminateAgent(agentId string) error {
	return agents.Terminate(agentId)
}
