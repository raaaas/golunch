// Package proxy resolves which proxy a run gets, and renders it as environment
// variables. It is env-var injection only: no server, no traffic interception,
// no base-URL rewriting, no MITM.
//
// Precedence, highest wins: the command-line flag, then the instance's own
// metadata, then the global default profile, then the host environment left
// untouched. Resolve reports which layer won, because "why is this not going
// through my proxy" is unanswerable without it.
//
// Both cases of every variable are always written after deleting all eight
// (HTTP_PROXY/http_proxy, HTTPS_PROXY/https_proxy, ALL_PROXY/all_proxy,
// NO_PROXY/no_proxy): some Node libraries read only lowercase, and divergent
// duplicates produce half-proxied traffic. Spec "none" force-unsets all eight,
// which is the only way to beat a proxy inherited from the host. Floor contributes
// the loopback range and the local link-local metadata address, so an agent
// talking to a local server is never sent through the tunnel.
package proxy
