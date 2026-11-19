package setup

import (
	"reflect"
	"testing"
)

func TestCurrentMaintenancePreservesQualifiedRoleAndReleaseArguments(t *testing.T) {
	tail := []string{"--product-root", "/synthetic/Developer/product", "--installed-manifest", "manifest", "--installed-client-inputs", "inputs", "--experimental-seed-sha256", "role", "--experimental-installed-root", "/synthetic/Developer/product"}
	args, err := maintenanceLaunchArguments(append([]string{"serve"}, tail...), "maintenance-status", "")
	if err != nil || args[0] != "maintenance-status" || !reflect.DeepEqual(args[1:], tail) {
		t.Fatalf("maintenance lost original qualified role argv: %q %v", args, err)
	}
	if _, err := maintenanceLaunchArguments([]string{"other"}, "maintenance-status", ""); err == nil {
		t.Fatal("unqualified maintenance accepted")
	}
}
