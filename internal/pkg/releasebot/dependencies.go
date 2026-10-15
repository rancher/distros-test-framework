package releasebot

import (
	"errors"
	"fmt"
	"strings"
)

// validateDependencies rejects duplicate job identities (product, version, name).
func validateDependencies(jobs []JenkinsJob) error {
	var problems []string
	deps := map[string][]string{}
	for i := range jobs {
		k := jobs[i].key()
		if _, dup := deps[k]; dup {
			problems = append(problems,
				fmt.Sprintf("duplicate job %s %s %q", jobs[i].Product, jobs[i].Version, jobs[i].Name))
			continue
		}
		deps[k] = jobs[i].depKeys()
	}

	for i := range jobs {
		for n, k := range jobs[i].depKeys() {
			if _, ok := deps[k]; !ok {
				problems = append(problems, fmt.Sprintf("%s %s depends on unknown job %q",
					jobs[i].Name, jobs[i].Version, jobs[i].DependsOn[n]))
			}
		}
	}

	if hasCycle(deps) {
		problems = append(problems, "dependency cycle between jobs")
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}

	return nil
}

// hasCycle runs Kahn's algorithm: whatever cannot be ordered is on (or behind) a cycle.
// References to unknown keys are ignored here; validateDependencies reports them.
func hasCycle(deps map[string][]string) bool {
	indegree := map[string]int{}
	dependents := map[string][]string{}
	for k, ds := range deps {
		for _, d := range ds {
			if _, ok := deps[d]; ok {
				indegree[k]++
				dependents[d] = append(dependents[d], k)
			}
		}
	}
	var queue []string
	for k := range deps {
		if indegree[k] == 0 {
			queue = append(queue, k)
		}
	}
	ordered := 0
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		ordered++
		for _, d := range dependents[k] {
			indegree[d]--
			if indegree[d] == 0 {
				queue = append(queue, d)
			}
		}
	}

	return ordered < len(deps)
}
