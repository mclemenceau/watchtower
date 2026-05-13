package main

import "go.temporal.io/sdk/workflow"

func StatusTableWorkflow(ctx workflow.Context) error  { return nil }
func ChangeWatchWorkflow(ctx workflow.Context) error  { return nil }
func QueryWorkflow(ctx workflow.Context, _ string) (string, error) { return "", nil }
