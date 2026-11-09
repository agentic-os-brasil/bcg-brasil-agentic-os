package baseruntime

import (
	"bytes"
	_ "embed"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/hookpolicy"
	maintenance "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenancecatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimecap"
)

//go:embed capabilities.json
var capabilitiesJSON []byte

//go:embed hook-policy.json
var hookPolicyJSON []byte

//go:embed orientation.md.tmpl
var orientationTemplate []byte

//go:embed maintenance.json
var maintenanceJSON []byte

func Manifest() (runtimecap.Manifest, error) {
	return runtimecap.Parse(bytes.NewReader(capabilitiesJSON))
}

func HookPolicy() (hookpolicy.Policy, error) {
	return hookpolicy.Parse(bytes.NewReader(hookPolicyJSON))
}

func OrientationTemplate() []byte {
	return append([]byte(nil), orientationTemplate...)
}

func Maintenance() (maintenance.Catalog, error) {
	return maintenance.Parse(bytes.NewReader(maintenanceJSON))
}

// Files returns detached copies of the finite canonical runtime payload.
func Files() map[string][]byte {
	return map[string][]byte{
		"capabilities.json":   append([]byte(nil), capabilitiesJSON...),
		"hook-policy.json":    append([]byte(nil), hookPolicyJSON...),
		"orientation.md.tmpl": append([]byte(nil), orientationTemplate...),
		"maintenance.json":    append([]byte(nil), maintenanceJSON...),
	}
}
