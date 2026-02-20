// Package registry defines the interface for copt discovery.
// Phase 1: FileRegistry reads /run/nile/*/endpoint.sock.
// Phase 2: DivanRegistry (centralized) or ScattercastRegistry (decentralized).
package registry

// CoptInfo describes a registered copt.
type CoptInfo struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"` // e.g. unix:///run/nile/my-copt/endpoint.sock
}

// Registry provides discovery of copts.
type Registry interface {
	// Register advertises a copt's endpoint.
	Register(name string, endpoint string) error

	// Lookup returns the endpoint for a copt by name.
	Lookup(name string) (string, error)

	// List returns all known copts.
	List() ([]CoptInfo, error)

	// Deregister removes a copt's registration.
	Deregister(name string) error
}
