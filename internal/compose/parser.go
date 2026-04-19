package compose

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Parse decodes a compose YAML document and validates it.
func Parse(data []byte) (File, error) {
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("compose parse: %w", err)
	}
	if err := validate(f); err != nil {
		return File{}, err
	}
	return f, nil
}

func validate(f File) error {
	if f.Version == "" {
		return fmt.Errorf("compose: missing version field")
	}
	if f.Version != "1" {
		return fmt.Errorf("compose: unsupported version %q (expected \"1\")", f.Version)
	}
	if len(f.Services) == 0 {
		return fmt.Errorf("compose: at least one service required")
	}
	for name, svc := range f.Services {
		if name == "" {
			return fmt.Errorf("compose: service name must not be empty")
		}
		if svc.Image == "" {
			return fmt.Errorf("compose: service %q missing image", name)
		}
		for _, dep := range svc.DependsOn {
			if _, ok := f.Services[dep]; !ok {
				return fmt.Errorf("compose: service %q depends_on unknown service %q", name, dep)
			}
		}
		for _, net := range svc.Networks {
			if _, ok := f.Networks[net]; !ok {
				return fmt.Errorf("compose: service %q references unknown network %q", name, net)
			}
		}
	}
	return nil
}
