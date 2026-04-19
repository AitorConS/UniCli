package compose

import (
	"fmt"
	"sort"
)

// TopologicalSort returns service names in dependency order (dependencies
// first). Returns an error if a dependency cycle is detected.
func TopologicalSort(services map[string]Service) ([]string, error) {
	inDegree := make(map[string]int, len(services))
	dependents := make(map[string][]string, len(services))

	for name := range services {
		inDegree[name] = 0
	}
	for name, svc := range services {
		for _, dep := range svc.DependsOn {
			dependents[dep] = append(dependents[dep], name)
			inDegree[name]++
		}
	}

	queue := make([]string, 0, len(services))
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	order := make([]string, 0, len(services))
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		deps := dependents[n]
		sort.Strings(deps)
		for _, dep := range deps {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
		sort.Strings(queue)
	}

	if len(order) != len(services) {
		return nil, fmt.Errorf("compose: dependency cycle detected")
	}
	return order, nil
}
