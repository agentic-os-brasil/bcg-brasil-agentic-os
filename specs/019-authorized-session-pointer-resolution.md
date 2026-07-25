# Spec 019 - Authorized session-pointer resolution

Status: owner-context resolution implemented; atlas, managed skills and memory
remain unavailable.

The Session Packet exposes pointers, not source bodies. `bcgos session resolve`
is an explicit follow-up read with a declared purpose and byte budget. It only
accepts `purpose=session`, only resolves pointers currently exposed by the
packet's owner context, rejects path traversal and returns `budget_exceeded`
instead of reading beyond the requested bound.

It is not a hook operation. Native hooks inject the bounded packet and return;
the agent may request an authorized pointer only when it needs its source.
Sensitive or unreviewed facets remain absent from the packet and cannot be
resolved through this command.
