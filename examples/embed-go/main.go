// Command embed-go shows how to embed Skein into a Go orchestrator or harness
// through the public API. Non-Go hosts embed the CLI binary instead — the
// manifest and report formats are the same.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	skein "github.com/Balanov/skein"
)

func main() {
	manifest := skein.Manifest{
		Version: 1,
		Workers: 2,
		Blocks: map[string]skein.Block{
			"audit": {
				Argv:      []string{"python", "-c", "import sys; print(sys.stdin.read().upper(), end='')"},
				TimeoutMS: 5000,
			},
		},
		Tasks: []skein.Task{
			{ID: "a", Block: "audit", Input: "agent draft"},
			{ID: "b", Block: "audit", Needs: []string{"a"}, StdinFrom: "a"},
		},
	}
	report, err := skein.Run(context.Background(), manifest, skein.Options{
		StateDir:     ".skein",
		Workers:      2,
		IncludeTasks: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("status:", report.Status)
	for id, task := range report.Tasks {
		fmt.Println("task:", id, task.Status, task.OutputPath)
	}
	if report.Status != skein.Succeeded {
		os.Exit(3)
	}
}
