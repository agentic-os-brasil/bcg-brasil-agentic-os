package nativeagentflow

// BoundSnapshot exposes only metadata admitted by the canonical bound reader.
// It creates no store or child, artifact, completion or review authority.
func (store Store) BoundSnapshot(session string) (State, error) {
	return store.readBound(session)
}
