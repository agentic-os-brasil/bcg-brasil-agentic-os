package zipruntime

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"

func casePathAllowed(root, path string) bool { return workspacepolicy.CasePathAllowed(root, path) }

func resolveProspective(path string) (string, error) { return workspacepolicy.ResolveProspective(path) }
