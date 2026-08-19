// keys.go exposes key matching utilities for mapping TermUI keyboard events to application actions.
package main

import "github.com/edsilegx/ctop/pkg/keys"

// IsKeyMatch evaluates whether a given TermUI key ID matches a registered logical action.
var IsKeyMatch = keys.IsKeyMatch
