package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

func LoadManifest(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if decoder.More() {
		return Manifest{}, errors.New("decode manifest: multiple JSON values")
	}
	return manifest, Validate(manifest)
}

func Validate(manifest Manifest) error {
	if manifest.Version != 1 {
		return fmt.Errorf("unsupported manifest version %d", manifest.Version)
	}
	if len(manifest.Tasks) == 0 {
		return errors.New("manifest contains no tasks")
	}
	if manifest.Workers < 0 || manifest.Workers > 65536 {
		return errors.New("workers must be between 0 and 65536")
	}
	if manifest.Limits.MaxTasks < 0 || manifest.Limits.MaxPasses < 0 {
		return errors.New("limits cannot be negative")
	}
	if manifest.Limits.MaxTasks > 0 && len(manifest.Tasks) > manifest.Limits.MaxTasks {
		return fmt.Errorf("task limit %d exceeded by %d tasks", manifest.Limits.MaxTasks, len(manifest.Tasks))
	}
	for name, block := range manifest.Blocks {
		if strings.TrimSpace(name) == "" {
			return errors.New("block name cannot be empty")
		}
		if len(block.Argv) == 0 || strings.TrimSpace(block.Argv[0]) == "" {
			return fmt.Errorf("block %q has no executable", name)
		}
		if block.Passes < 0 {
			return fmt.Errorf("block %q has negative passes", name)
		}
		if manifest.Limits.MaxPasses > 0 && block.Passes > manifest.Limits.MaxPasses {
			return fmt.Errorf("block %q exceeds pass limit %d", name, manifest.Limits.MaxPasses)
		}
		if block.TimeoutMS < 0 {
			return fmt.Errorf("block %q has negative timeout", name)
		}
		for _, entry := range block.Env {
			if !strings.Contains(entry, "=") || strings.HasPrefix(entry, "=") {
				return fmt.Errorf("block %q has invalid environment entry %q", name, entry)
			}
		}
	}

	indices := make(map[string]int, len(manifest.Tasks))
	indegree := make(map[string]int, len(manifest.Tasks))
	dependents := make(map[string][]string, len(manifest.Tasks))
	for index, task := range manifest.Tasks {
		if strings.TrimSpace(task.ID) == "" {
			return fmt.Errorf("task %d has an empty id", index)
		}
		if _, exists := indices[task.ID]; exists {
			return fmt.Errorf("duplicate task id %q", task.ID)
		}
		indices[task.ID] = index
		indegree[task.ID] = len(task.Needs)
		if _, exists := manifest.Blocks[task.Block]; !exists {
			return fmt.Errorf("task %q references unknown block %q", task.ID, task.Block)
		}
		if task.Passes < 0 {
			return fmt.Errorf("task %q has negative passes", task.ID)
		}
		if manifest.Limits.MaxPasses > 0 && task.Passes > manifest.Limits.MaxPasses {
			return fmt.Errorf("task %q exceeds pass limit %d", task.ID, manifest.Limits.MaxPasses)
		}
		sources := 0
		if task.Input != "" {
			sources++
		}
		if task.InputFile != "" {
			sources++
		}
		if task.StdinFrom != "" {
			sources++
		}
		if sources > 1 {
			return fmt.Errorf("task %q has multiple stdin sources", task.ID)
		}
	}

	for _, task := range manifest.Tasks {
		seen := make(map[string]struct{}, len(task.Needs))
		for _, need := range task.Needs {
			if _, exists := indices[need]; !exists {
				return fmt.Errorf("task %q needs unknown task %q", task.ID, need)
			}
			if need == task.ID {
				return fmt.Errorf("task %q depends on itself", task.ID)
			}
			if _, duplicate := seen[need]; duplicate {
				return fmt.Errorf("task %q lists dependency %q twice", task.ID, need)
			}
			seen[need] = struct{}{}
			dependents[need] = append(dependents[need], task.ID)
		}
		if task.StdinFrom != "" {
			if _, ok := seen[task.StdinFrom]; !ok {
				return fmt.Errorf("task %q reads from %q without declaring it in needs", task.ID, task.StdinFrom)
			}
		}
	}

	queue := make([]string, 0, len(manifest.Tasks))
	for _, task := range manifest.Tasks {
		if indegree[task.ID] == 0 {
			queue = append(queue, task.ID)
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, child := range dependents[id] {
			indegree[child]--
			if indegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}
	if visited != len(manifest.Tasks) {
		return errors.New("task graph contains a cycle")
	}
	return nil
}

func WorkerCount(manifest Manifest, override int) int {
	if override > 0 {
		return override
	}
	if manifest.Workers > 0 {
		return manifest.Workers
	}
	return runtime.GOMAXPROCS(0)
}

func PassCount(task Task, block Block) int {
	if task.Passes > 0 {
		return task.Passes
	}
	if block.Passes > 0 {
		return block.Passes
	}
	return 1
}
