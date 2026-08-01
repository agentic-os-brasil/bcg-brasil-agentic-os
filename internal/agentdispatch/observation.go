package agentdispatch

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/selfmodel"

// InteractionRecorder is a Maestro-owned post-interaction seam. It runs for
// every loop, regardless of whether Walter was selected, and persists only
// the selfmodel evaluator's authenticated material signal.
type InteractionRecorder struct {
	Store selfmodel.Store
}

func (recorder InteractionRecorder) AfterInteraction(input selfmodel.InteractionInput) (selfmodel.Observation, error) {
	observation, persist, err := selfmodel.EvaluateInteraction(input)
	if err != nil {
		return selfmodel.Observation{}, err
	}
	if persist {
		if err := recorder.Store.Append(observation); err != nil {
			return selfmodel.Observation{}, err
		}
	}
	return observation, nil
}
