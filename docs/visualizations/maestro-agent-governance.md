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
  Maestro --> Material{Walter required?}
  Material --> Walter[Walter review]
  Material --> Deliver[delivery]
  Walter --> Maestro
  Maestro --> Deliver
  Deliver --> User
```

The first decision is `account_assistance`: account-assisted work has both
framing and return validation; direct Case work has neither. The second is
`walter_required`: material or ambiguous work goes to Walter, while a
low-materiality skip requires a Maestro reason code and evidence. The two
decisions are never collapsed into one depth profile.

Case methods remain local skills. PA Expert is the sole centrally versioned
FPA/IPA advisory authority. Walter is internal and tool-free. Darwin handles
scoped health/governance maintenance only and cannot approve itself or mutate
live policy.
