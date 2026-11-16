package pii

import "os"

// The user-cache directory inherits the user's profile ACL on Windows.
func cacheDirectoryOwned(info os.FileInfo) bool { return info.IsDir() }

func cacheDirectoryPrivate(info os.FileInfo) bool { return info.IsDir() }

func cacheParentTrusted(info os.FileInfo) bool { return info.IsDir() }
