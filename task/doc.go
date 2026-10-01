// Package task defines the taskfile schema and the fan-out bookkeeping.
//
// A taskfile is N prompts against M instances, with defaults that each task may
// override: an instance, a model, a parallelism, a timeout. The `prompts` form
// expands one entry into several runs. Duration accepts "90s", "2m", or bare
// seconds, because a hand-written JSON file is not a Go program.
//
// This package holds no runner. Outcome and Request are the shapes the runner
// fills in, which is what lets a sweep keep going after one prompt fails — a bad
// prompt in a batch of fifty is not a reason to lose the other forty-nine — while
// still exiting nonzero.
package task
