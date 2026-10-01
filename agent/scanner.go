package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// MaxLineBytes bounds one NDJSON record. The kilo/opencode bundle caps a
// single stdout write at 10 MiB (an embedded tool result or file attachment
// can be that large), so a 4 KiB default scanner buffer would fail the whole
// run with "token too far past newline" on a legitimate line.
const MaxLineBytes = 16 << 20

// ParseLine turns one wire-format record into zero or more normalized events.
// ok=false means "this is not a record I understand" and the scanner will
// count it as skipped rather than treating it as a fatal parse error.
type ParseLine func(line []byte) (events []Event, ok bool)

// Stats summarizes a completed scan.
type Stats struct {
	Lines   int
	Events  int
	Skipped int
	// LongestLine is reported by `golunch doctor` when a run produced no
	// events: an oversized single record is the most likely cause.
	LongestLine int
}

// ErrNoEvents means the stream produced nothing parseable. A driver that
// returns this after a zero-exit process is the failure mode the original
// sample code had: wrong struct shape, empty answer, no error.
var ErrNoEvents = errors.New("agent produced no parseable events")

// Scan reads NDJSON from r and pushes every decoded event to out. It stops
// reading as soon as out is closed by the consumer, so a caller that wants
// only the first sentence does not have to drain an agent's whole transcript.
//
// Non-JSON lines are skipped rather than fatal. This is not laxness: kilo
// prints an ASCII art banner on stdout and `INFO <ts> ... service=default`
// diagnostics on stderr that can interleave with real records, so a strict
// parser fails a perfectly good run.
func Scan(r io.Reader, parse ParseLine, out chan<- Event) (Stats, error) {
	var st Stats
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), MaxLineBytes)
	sc.Split(dropTrailingCR)

	for sc.Scan() {
		line := sc.Bytes()
		st.Lines++
		if n := len(bytes.TrimSpace(line)); n > st.LongestLine {
			st.LongestLine = n
		}
		if !looksLikeJSON(line) {
			st.Skipped++
			continue
		}
		// Copy: Scanner reuses its buffer and events outlive this iteration.
		data := append([]byte(nil), line...)
		evs, ok := parse(data)
		if !ok || len(evs) == 0 {
			st.Skipped++
			continue
		}
		for _, e := range evs {
			st.Events++
			out <- e
		}
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return st, err
		}
		if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.EOF) {
			return st, nil
		}
		return st, err
	}
	return st, nil
}

// dropTrailingCR keeps CRLF-terminated records (some Node writers use them)
// from smuggling a \r into the last field of every event.
func dropTrailingCR(data []byte, atEOF bool) (int, []byte, error) {
	if advance, tl, err := bufio.ScanLines(data, atEOF); err != nil {
		return advance, tl, err
	} else if len(tl) > 0 && tl[len(tl)-1] == '\r' {
		return advance, tl[:len(tl)-1], nil
	} else {
		return advance, tl, nil
	}
}

// looksLikeJSON is a cheap gate: real records are objects. Arrays and scalars
// are not part of any agent's documented stream, and rejecting them here
// keeps a stray `[` progress bar from reaching the driver's unmarshal.
func looksLikeJSON(line []byte) bool {
	t := bytes.TrimLeft(line, " \t")
	return len(t) > 0 && t[0] == '{'
}

// compactRaw stores the original record without surrounding whitespace.
func compactRaw(line []byte) json.RawMessage {
	t := bytes.TrimSpace(line)
	if len(t) == 0 {
		return nil
	}
	return json.RawMessage(append([]byte(nil), t...))
}
