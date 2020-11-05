// +build !windows

package env

import (
	"fmt"

	"github.com/newrelic/newrelic-diagnostics-cli/logger"
	"github.com/newrelic/newrelic-diagnostics-cli/tasks"
	"github.com/shirou/gopsutil/process"
)

// BaseEnvDetectLanguage gets information on the programming language by looking at the processes
type BaseEnvDetectLanguage struct {
}

// Identifier - This returns the Category, Subcategory and Name of each task
func (t BaseEnvDetectLanguage) Identifier() tasks.Identifier {
	return tasks.IdentifierFromString("Base/Env/DetectLanguage")
}

// Explain - Returns the help text for each individual task
func (t BaseEnvDetectLanguage) Explain() string {
	return "Detect programming languages"
}

// Dependencies - Returns the dependencies for ech task.
func (t BaseEnvDetectLanguage) Dependencies() []string {
	return []string{}
}

// Execute - The core work within each task
func (t BaseEnvDetectLanguage) Execute(options tasks.Options, upstream map[string]tasks.Result) tasks.Result {

	apmLanguages := []string{"java", "node", "ruby", "python", "dotnet", "w3wp.exe"} // the latter represents .NET framework
	foundProcesses := make(map[string][]process.Process)
	for _, lang := range apmLanguages {
		processes, err := tasks.FindProcessByName(lang)
		if err != nil {
			return tasks.Result{
				Status:  tasks.Error,
				Summary: fmt.Sprintf("we ran into an error while trying to find a process running for %s: %s", lang, err),
				Payload: foundProcesses,
			}
		}
		if processes == nil {
			logger.Debugf("no processes found for: %s", lang)
		} else {
			foundProcesses[lang] = processes
		}
	}

	if len(foundProcesses) > 0 {
		return tasks.Result{
			Status:  tasks.Info,
			Summary: "Collected language information",
			Payload: foundProcesses,
		}
	}

	return tasks.Result{
		Status:  tasks.Warning,
		Summary: "we did not find any references to the programming language being used in this environment",
	}

}
