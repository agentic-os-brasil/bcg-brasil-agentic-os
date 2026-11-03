package nativeagentflow

// BoundSnapshot returns only the metadata admitted by the canonical bound
// reader. It creates no store and grants no child, artifact or review authority.
// Qualification consumes this view rather than decoding a weaker second copy.
func (store Store) BoundSnapshot(session string) (State, error) {
	return store.readBound(session)
}
