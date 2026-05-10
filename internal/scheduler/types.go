package scheduler

type Resources struct {
	CPU       int `json:"cpu"`
	MemoryMiB int `json:"memoryMiB"`
}

func (r Resources) Fits(required Resources) bool {
	return r.CPU >= required.CPU && r.MemoryMiB >= required.MemoryMiB
}

func (r Resources) Add(other Resources) Resources {
	return Resources{CPU: r.CPU + other.CPU, MemoryMiB: r.MemoryMiB + other.MemoryMiB}
}

func (r Resources) Sub(other Resources) Resources {
	return Resources{CPU: r.CPU - other.CPU, MemoryMiB: r.MemoryMiB - other.MemoryMiB}
}

type Workflow struct {
	Name  string     `json:"name"`
	Tasks []TaskSpec `json:"tasks"`
}

type TaskSpec struct {
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	Command   []string  `json:"command"`
	DependsOn []string  `json:"dependsOn"`
	Resources Resources `json:"resources"`
}

type Cluster struct {
	Name      string    `json:"name"`
	Context   string    `json:"context"`
	Namespace string    `json:"namespace"`
	Capacity  Resources `json:"capacity"`
}

type TaskStatus string

const (
	TaskPending   TaskStatus = "Pending"
	TaskReady     TaskStatus = "Ready"
	TaskRunning   TaskStatus = "Running"
	TaskSucceeded TaskStatus = "Succeeded"
	TaskFailed    TaskStatus = "Failed"
)

type TaskResult struct {
	Name    string
	Status  TaskStatus
	Cluster string
}

type WorkflowResult struct {
	WorkflowName string
	Tasks        []TaskResult
}
