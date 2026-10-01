// Package atomicfile writes a file by writing a sibling temp file and renaming it
// into place.
//
// metadata.toml and a driver's config are both hand-editable and both read at
// every launch, so a crash halfway through a save has to leave the previous
// contents intact rather than a truncated file that the next run cannot parse.
package atomicfile
