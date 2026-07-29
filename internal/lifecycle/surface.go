package lifecycle

// Surface describes one runtime binding without asserting that the binding
// was observed natively. Adapters may report configured or blocked surfaces;
// only a fresh native observation can qualify the corresponding capability.
type Surface struct {
	SemanticEvent     string `json:"semantic_event"`
	NativeBinding     string `json:"native_binding"`
	Implementation    string `json:"implementation"`
	EvidenceClass     string `json:"evidence_class,omitempty"`
	NativeObservation string `json:"native_observation"`
	CapabilityState   string `json:"capability_state"`
	Blocker           string `json:"blocker,omitempty"`
}
