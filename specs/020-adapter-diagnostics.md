# Spec 020 - Adapter diagnostics

Status: local configuration inspection implemented; runtime execution evidence
and conformance receipts pending.

`bcgos doctor` reports runtime executable discovery and workspace adapter
configuration as distinct checks. A configured adapter means that Maestro's
owned local entry exists; it does not claim the runtime trusted it, invoked it
or that a lifecycle capability is available. Lifecycle receipts are reported as
`adapter-command` evidence: they prove only that the bounded Maestro command
ran and never native-session origin. Capability state remains derived from the
canonical manifest until the runtime/platform protocol in Spec 021 establishes
qualified native evidence. See Spec 035 for the executable evidence matrix.
