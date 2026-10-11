//go:build !unix

package cmd

// Non-Unix builds have no O_NOFOLLOW. Symlinks are still rejected by Lstat.
const lockNoFollow = 0
