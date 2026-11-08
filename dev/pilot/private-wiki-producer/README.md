# Compatible private-wiki producer witness

This development-only executable supplies the real second process required by
`TestPrivateWikiCompatibleProducerAcrossActualBinaries`. It is a compatible
current-source variant, never a historical release. It is not distributed.

The executable accepts exactly `ABSOLUTE_ROOT ABSOLUTE_CONFIG`. The configuration
contains the test's unchanged `policy`, `budgets` and `freshness`. It strictly
decodes those fields and calls `Engine.CompilePrivateWiki` for the synthetic
`case-a` workspace. Standard output is the actual `PrivateWikiResult` JSON.
It does not prepare authority, seed sources or manufacture a graph or manifest.

Build with `python3 dev/pilot/private-wiki-producer/build.py ABSOLUTE_NEW_DIRECTORY
--source-revision FULL_SOURCE_SHA`. The builder uses the current Go target and
retains one description-only overlay, its mapping, build logs and
`producer-build.json`. It changes only the index description to
`Compatible producer-only description`; schema, profile, compiler version and
configuration remain unchanged. An existing output directory is refused.

Set `MAESTRO_PRIVATE_WIKI_COMPAT_PRODUCER` to the resulting absolute executable
path and run only the existing compatibility test. The test verifies actual
producer output, SHA256 provenance, current-reader acceptance, unchanged
historical manifest bytes and a genuine subsequent current-build compilation.
A cross-built PE is not native Windows proof.

For a checkout, the declared revision must equal HEAD. The retained witness-input
hashes describe added or uncommitted helper inputs; they do not claim an arbitrary
working tree equals that commit. A staged source archive has no Git metadata, so
its caller must bind `--source-revision` to the already sealed archive.
