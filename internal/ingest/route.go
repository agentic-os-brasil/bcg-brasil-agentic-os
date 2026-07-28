package ingest

import "errors"

type PrimaryState string

const (
	PrimaryReady       PrimaryState = "ready"
	PrimaryUnavailable PrimaryState = "unavailable"
	PrimaryUnsupported PrimaryState = "unsupported"
	PrimaryDegraded    PrimaryState = "degraded"
)

type RouteDecision struct {
	Adapter string
	Route   string
	Reason  string
}

// SelectFallback is the core-owned decision point for deterministic fallback
// routing. Providers do not infer this from source content or choose a route
// by themselves.
func SelectFallback(primary PrimaryState, requestedAdapter string) (RouteDecision, error) {
	if requestedAdapter != "markitdown" {
		return RouteDecision{}, errors.New("requested ingestion adapter is not registered")
	}
	switch primary {
	case PrimaryUnavailable:
		return RouteDecision{
			Adapter: "markitdown",
			Route:   "markitdown_fallback",
			Reason:  "the primary Docling route is unavailable",
		}, nil
	case PrimaryUnsupported:
		return RouteDecision{
			Adapter: "markitdown",
			Route:   "markitdown_fallback",
			Reason:  "the primary Docling route does not support this source",
		}, nil
	case PrimaryDegraded:
		return RouteDecision{
			Adapter: "markitdown",
			Route:   "markitdown_fallback",
			Reason:  "the primary Docling route produced a degraded result",
		}, nil
	case PrimaryReady:
		return RouteDecision{}, errors.New("fallback route is not selected while the primary route is ready")
	default:
		return RouteDecision{}, errors.New("primary ingestion route state is invalid")
	}
}
