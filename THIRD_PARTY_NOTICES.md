# Third-party notices

The offline notice shipped with the application is
[resources/THIRD_PARTY_NOTICES.md](resources/THIRD_PARTY_NOTICES.md).
The complete license resource package is indexed in
[resources/licenses/README.md](resources/licenses/README.md), with source records,
original license texts and an explicit list of unresolved items.

MyGo automatically packages the contents of `resources/`. On Windows these
notices appear beside the executable; on macOS they appear in
`Zeron.app/Contents/Resources/`. The release verifier checks every listed file
and its checksum before producing release archives.

The original Zeron MIT notice is retained for copied source material and assets.
This inventory does not assign a new license or a copyright holder to newly
authored replica code; that outstanding project decision is recorded in
[OPEN-ITEMS.md](resources/licenses/OPEN-ITEMS.md).

After intentionally updating reviewed license texts or their source records, run
`go run ./scripts/license-manifest.go` and `go test ./scripts/licenseresources`.
The manifest generator requires a source mapping for every file and performs no
automatic license download or license-name-to-text substitution.
