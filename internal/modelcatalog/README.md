# models.dev local metadata cache

`models-dev.json` is the unmodified response downloaded from
`https://models.dev/api.json` on 2026-10-30. SHA-256:
`4e9ba4423a35cc60d18b9aa5bda8a67895090a814d496ee5bc6248716d5a7d97`.
It is bundled only as offline metadata for user-configured model forms; providers
and models in this file are never added to the executable catalog automatically.
The original MIT license is shipped in `resources/licenses/models-dev/LICENSE.txt`.

The business host writes the seed to its data directory on first launch.
`Lookup` reads the index built from that local file and performs no HTTP.
One host-owned background loop refreshes every three hours, keeping conditional
HTTP validators and the last attempted time in `models-dev-state.json`.
A complete nonempty response is validated before atomic replacement. Failed
responses preserve the good JSON and index. A refresh never locks out lookups.
Shutdown cancels and joins the refresher independently of business task waits.

Matching uses exact request IDs and source-qualified IDs, preferring a provider
with the configured Base URL host. Equal-ranked candidates are ordered by
provider ID and model key; the first is applied deterministically. Missing models
or lookup failures leave the form unchanged without status copy. No provider credential or entered model ID is
sent to models.dev. The request is always for the complete public JSON file.

Metadata modifies only an explicit add/edit draft. Opening a saved model keeps
saved values; automatic configuration is always enabled without a switch.
Resetting or changing an ID starts a fresh lookup, and draft identity/revision
checks prevent old async results from overriding newer edits.
