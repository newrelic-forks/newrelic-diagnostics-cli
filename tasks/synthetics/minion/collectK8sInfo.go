package minion

import (
	"os"
	"os/exec"

	"github.com/newrelic/newrelic-diagnostics-cli/tasks"
)

// SyntheticsMinionCollectK8sInfo - This struct defined the sample plugin which can be used as a starting point
type SyntheticsMinionCollectK8sInfo struct {
}

// Identifier - This returns the Category, Subcategory and Name of each task
func (p SyntheticsMinionCollectK8sInfo) Identifier() tasks.Identifier {
	return tasks.IdentifierFromString("Synthetics/Minion/CollectK8sInfo")
}

// Explain - Returns the help text for each individual task
func (p SyntheticsMinionCollectK8sInfo) Explain() string {
	return "Collect Kubernetes Info to assist in troubleshooting Kubernetes clusters"
}

// Dependencies - Returns the dependencies for each task.
func (p SyntheticsMinionCollectK8sInfo) Dependencies() []string {
	return []string{}
}

// Execute - The core work within each task
func (p SyntheticsMinionCollectK8sInfo) Execute(options tasks.Options, upstream map[string]tasks.Result) tasks.Result {

	//bash -c "$(curl -s https://raw.githubusercontent.com/keegoid-nr/cki/v0.6/cki.sh)"

	ckiScript := "https://raw.githubusercontent.com/keegoid-nr/cki/v0.6/cki.sh"
	cmd := exec.Command("bash", "-c", "curl -s "+ckiScript+" | bash")
	// cmd := exec.Command("bash", "-c", "../scripts/cki.sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil {
		return tasks.Result{
			Status:  tasks.Error,
			Summary: "Unable to run CKI script: " + err.Error(),
		}
	}

	return tasks.Result{
		Status:  tasks.Info,
		Summary: "Logs and files collected",
	}
}
