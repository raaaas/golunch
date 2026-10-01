// Package config reads ~/.golunch/config.toml and resolves where golunch keeps
// its data.
//
// Load is deliberately quiet about a missing file — a fresh machine has no
// config and that is not an error — but loud about a malformed one.
//
// DataRoot honours GOLUNCH_ROOT, and refuses to nest: if $HOME points inside
// another isolation tool's instance, the data root is resolved from the real home
// instead, and the NestingWarning it returns is what makes `golunch doctor` say so
// out loud rather than let ~/.golunch silently live inside that instance and be
// deleted with it.
package config
