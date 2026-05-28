package main

import "gopkg.in/yaml.v3"

// parseDeploymentCredentials parses a battlegroup YAML and returns the DB user
// and password from spec.database.template.spec.deployment.spec.{user,password}.
func parseDeploymentCredentials(data []byte) (user, pass string) {
	var root struct {
		Spec struct {
			Database struct {
				Template struct {
					Spec struct {
						Deployment struct {
							Spec struct {
								User     string `yaml:"user"`
								Password string `yaml:"password"`
							} `yaml:"spec"`
						} `yaml:"deployment"`
					} `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"database"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return "", ""
	}
	s := root.Spec.Database.Template.Spec.Deployment.Spec
	if s.User == "" {
		s.User = "dune"
	}
	return s.User, s.Password
}
