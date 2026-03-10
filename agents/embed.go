package agents

import "embed"

//go:embed *.json hooks/*
var AgentsFS embed.FS
