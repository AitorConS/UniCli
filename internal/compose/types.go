package compose

// File is a parsed uni compose file.
type File struct {
	// Version must be "1".
	Version string `yaml:"version"`
	// Services maps service name to its definition.
	Services map[string]Service `yaml:"services"`
	// Networks maps network name to its definition.
	Networks map[string]Network `yaml:"networks"`
}

// Service describes a single unikernel service.
type Service struct {
	// Image is a name:tag reference or file path.
	Image string `yaml:"image"`
	// Memory is the QEMU memory string (e.g. "256M").
	Memory string `yaml:"memory"`
	// CPUs is the number of virtual CPUs; 0 uses QEMU default.
	CPUs int `yaml:"cpus"`
	// DependsOn lists services that must start before this one.
	DependsOn []string `yaml:"depends_on"`
	// Networks lists logical network names to attach.
	Networks []string `yaml:"networks"`
	// Environment is a list of KEY=VALUE pairs.
	Environment []string `yaml:"environment"`
}

// Network describes a logical network.
type Network struct {
	// Driver is the network driver; only "bridge" is supported.
	Driver string `yaml:"driver"`
}

// State records running VM IDs for a compose project.
type State struct {
	// Project is the compose project name (directory basename).
	Project string `json:"project"`
	// Services maps service name to VM ID.
	Services map[string]string `json:"services"`
}
