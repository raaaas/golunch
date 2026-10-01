// Package execd starts one child, streams its normalized output, and guarantees
// it does not outlive the run.
//
// The child gets its own process group, so a timeout or a cancel kills the agent
// and everything the agent spawned. That is the whole reason this package exists:
// a killed launcher that leaves a live agent holding a lock is worse than no
// launcher, and an orphan holding an instance lock breaks the zero-daemon
// guarantee.
//
// Every line is decoded through the driver's ParseLine as it arrives and handed
// to Spec.OnEvent, and the transcript is appended to the instance's raw log
// line-by-line. Output is never buffered until exit: a five-minute run that
// prints nothing until it finishes is not usable.
package execd
