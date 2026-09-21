package main

import (
	"github.com/ipinfo/cli/lib/complete"
	"github.com/ipinfo/cli/lib/complete/predict"
	shrinkcmd "github.com/ipinfo/mmdbctl/mmdbshrink/cmd"
)

var completions = &complete.Command{
	Sub: map[string]*complete.Command{
		"read":       completionsRead,
		"import":     completionsImport,
		"export":     completionsExport,
		"diff":       completionsDiff,
		"metadata":   completionsMetadata,
		"verify":     completionsVerify,
		"completion": completionsCompletion,
		"shrink":     shrinkcmd.Completions(),
	},
	Flags: map[string]complete.Predictor{
		"--nocolor": predict.Nothing,
		"-h":        predict.Nothing,
		"--help":    predict.Nothing,
	},
}

func handleCompletions() {
	completions.Complete(progBase)
}
