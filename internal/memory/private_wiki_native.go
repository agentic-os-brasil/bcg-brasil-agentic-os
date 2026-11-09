package memory

import "errors"

// Wiki admission also requires the actual retained namespace operation. Ordinary
// memory admission alone cannot grant graph publication or retirement custody.
var ErrPrivateWikiNativeUnavailable = errors.New("private wiki native confinement is unqualified")

func (engine *Engine) privateWikiNativeAvailable(workspace string) error {
	custody, err := engine.holdPrivateWikiWorkspace(workspace)
	if err != nil {
		return err
	}
	custody()
	return nil
}
