# Opt-in Windows public-input construction proof

`TestPublicAcquisitionConstruction` is a package-local construction check for the
shared acquisition implementation. It deliberately uses a synthetic server and
bundle plus a fresh in-memory fixture key. Its public metadata is not a
participant release, installable carrier or production authority. Neither the
synthetic server nor any acquired Windows/Python/DLL code executes.

The default focused test run skips this check. Explicit modes are separate:

1. `preflight` accepts the previously reviewed public inventory, verifies the
   already constructed expected archive, generates one synthetic signed public
   carrier/recipe and calls read-only `Review`. Only public registry, metadata,
   signatures and the preflight receipt are retained; the private key exists
   only inside this process. Cache remains absent and no request is made.
2. Stop for independent inspection and separate authorization of the exact
   `preflight.json` SHA-256. The test itself cannot grant network authority.
3. `acquire` requires that exact approval digest and an unchanged, unexpired
   review and public metadata. It retains the held private-directory and
   transition-lock guards, stages one verified input, explicitly reviews the
   partial overall acquisition and calls the same private `prepare` function
   used after the application's native Windows guard. This exercises real
   public fetching, completed-input resume, canonical assembly and ordinary
   whole-release verification on the current host. It does not modify or claim
   execution of the exported Windows-only application entry.

The required environment names are declared at the start of the test. Paths must
be explicit and absolute; execution requires the separately approved Developer
artifact/cache locations. The inventory digest, final profile input pins and
expected runtime archive are closed test inputs. There is no URL, command,
resolver, dependency or authority override exposed to the product.

The fixed closure has 57 inputs and 132,964,926 expected asset bytes. A fresh
private cache preserves all partial and completed bytes without overwrites.
Exactly one completed input is reused during the staged resume; that input is
not downloaded twice. The shared production policy bounds the publisher hosts,
redirects, public DNS socket destinations, TLS, input bytes/hash and timeouts.
The proof adds a twenty-minute context and a trace budget of at most 228 HTTPS
connection acquisitions, records requests without sensitive redirect query
strings and verifies the original public carrier remains unchanged.

A failing attempt stops and preserves its cache. A later network attempt needs a
new explicitly inspected review; this test does not automatically retry or
refresh consent. Completed and partial files are not license acceptance or
native readiness evidence. Actual ordinary-account Windows acquisition,
placement, loader, conversion, AppContainer controls and client acceptance stay
separate qualification gates.
