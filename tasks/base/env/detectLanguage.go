// +build !windows

package env

import (
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
	var foundProcesses map[string][]process.Process
	for _, lang := range apmLanguages {
		processes, err := tasks.FindProcessByName(lang)
		if err != nil {
			logger.Infof("processes not found for %s: %s", lang, err)
		} else {
			foundProcesses[lang] = processes
		}
	}

	if foundProcesses != nil {
		return tasks.Result{
			Status:  tasks.Info,
			Summary: "Collected language information",
			Payload: foundProcesses,
		}
	}

	return tasks.Result{
		Status:  tasks.Warning,
		Summary: "we did not find any references to the programmin language being used for this app",
	}

}
