# Maestro agent governance

Maestro is the sole user-facing hub. It keeps one active spoke globally and
mediates every packet; agents never call one another or open nested work.

```mermaid
flowchart LR
  User --> Maestro
  Maestro --> Decision{two independent decisions}
  Decision --> Account[Client Account framing]
  Decision --> Case[Case execution]
  Account --> Maestro
  Maestro --> Case
  Case --> Maestro
  Maestro --> AccountValidation[validation iff framing used]
  AccountValidation --> Maestro
  Maestro --> Material{Yoda required?}
  Material --> Yoda[Yoda review]
  Material --> Deliver[delivery]
  Yoda --> Maestro
  Maestro --> Deliver
  Deliver --> User
```

```mermaid
flowchart LR
  Maestro --> Gamma[Gamma Guardian\nlongitudinal quality]
  Gamma --> Maestro
  Gamma -. "one authorized workspace/head per packet" .-> Evidence[bounded evidence]
  Gamma -. "no Case context, children, write or merge" .-> Boundary[fail-closed boundary]
```

The first decision is `account_assistance`: account-assisted work has both
framing and return validation; direct Case work has neither. The second is
`yoda_required`: material or ambiguous work goes to Yoda, while a
low-materiality skip requires a Maestro reason code and evidence. The two
decisions are never collapsed into one depth profile.

Case methods remain local skills. PA Expert is the sole centrally versioned
FPA/IPA advisory authority. Yoda is internal and tool-free. Darwin handles
scoped health/governance maintenance only and cannot approve itself or mutate
live policy. Gamma Guardian is a direct Maestro spoke for longitudinal code
quality; its rubric is a method inside that agent, and it never becomes a Case
child or inherits Case context. Local quality signals remain advisory until
independent native runtime evidence exists.
