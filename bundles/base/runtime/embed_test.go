package baseruntime

import "testing"

func TestEmbeddedMaintenanceCatalogIsAvailableThroughBaseRuntime(t *testing.T) {
	catalog, err := Maintenance()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Jobs) != 16 || catalog.CatalogState != "catalog_only" {
		t.Fatalf("embedded maintenance catalog = %#v", catalog)
	}
}
