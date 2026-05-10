package scheduler

import "testing"

func TestBuildGraphRejectsCycle(t *testing.T) {
	workflow := Workflow{
		Name: "cycle",
		Tasks: []TaskSpec{
			{Name: "a", DependsOn: []string{"b"}},
			{Name: "b", DependsOn: []string{"a"}},
		},
	}

	if _, err := buildGraph(workflow); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestBuildGraphRejectsUnknownDependency(t *testing.T) {
	workflow := Workflow{
		Name: "unknown",
		Tasks: []TaskSpec{
			{Name: "a", DependsOn: []string{"missing"}},
		},
	}

	if _, err := buildGraph(workflow); err == nil {
		t.Fatal("expected unknown dependency error")
	}
}
