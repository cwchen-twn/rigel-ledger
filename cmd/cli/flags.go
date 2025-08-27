package main

import "fmt"

type Command struct {
	Value string
}

func (c *Command) String() string {
	return c.Value
}

func (c *Command) Set(value string) error {
	switch value {
	case "create-user":
		c.Value = "create-user"
	default:
		return fmt.Errorf("invalid command: %s", value)
	}
	return nil
}

func (c *Command) Type() string {
	return "command"
}
