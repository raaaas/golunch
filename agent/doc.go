// Package agent is the driver registry: one data literal per agent CLI, holding
// how to build its argv, how to parse one line of its output, where it keeps
// credentials, and which host environment variables it cannot work without.
//
// There is no per-agent code. Adding claude or codex means adding a row, and
// everything above — run, task, doctor, the generated launcher — picks it up
// through Lookup.
//
// Headless output is decoded into a single Event type (text, thinking,
// tool_call, tool_result, usage, status, error, log) with the original line kept
// in Raw, so a schema golunch has never seen degrades to a log line instead of
// vanishing. An unknown driver producing zero events is a loud failure, not a
// silent success.
package agent
