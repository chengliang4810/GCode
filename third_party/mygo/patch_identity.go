package mygo

import (
	"encoding/hex"
	"fmt"
)

// SourcePatch identifies the reviewed local changes to the upstream framework.
// SourceSHA256 is populated by the release scripts after checking the source.
type SourcePatch struct {
	UpstreamVersion string
	Revision        string
	SourceSHA256    string
}

// These values are also recorded in the executable's Go linker settings.
// Development builds keep sourcePatchSHA256 empty; release verification rejects
// such builds and requires the complete fixed identity.
var sourcePatchRevision = "rcode-native-ui.24"
var sourcePatchSHA256 string
var sourcePatchIdentity string

// FrameworkPatch reports the framework's upstream version and local patch.
func FrameworkPatch() SourcePatch {
	return SourcePatch{UpstreamVersion: "v0.8.3", Revision: sourcePatchRevision, SourceSHA256: sourcePatchSHA256}
}

func init() {
	if sourcePatchRevision != "rcode-native-ui.24" {
		panic("mygo: unexpected local patch revision: " + sourcePatchRevision)
	}
	if sourcePatchSHA256 != "" {
		decoded, err := hex.DecodeString(sourcePatchSHA256)
		if err != nil || len(decoded) != 32 {
			panic(fmt.Sprintf("mygo: invalid local source fingerprint: %q", sourcePatchSHA256))
		}
	}
	if sourcePatchIdentity != "" && sourcePatchIdentity != "RCODE_MYGO_PATCH[v0.8.3|"+sourcePatchRevision+"|"+sourcePatchSHA256+"]" {
		panic("mygo: inconsistent framework release identity")
	}
}
