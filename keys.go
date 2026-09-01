// keys.go exposes key matching utilities for mapping TermUI keyboard events to application actions.
//
// Objective:
//
//	Provide a centralized bridge linking package main UI event handlers to the reusable keybinding registry in pkg/keys.
//
// Core Components:
//   - IsKeyMatch: Logical action evaluator mapping keys (e.g. "q", "<Escape>", "j", "k") to actions ("exit", "down", "up").
//
// Functionality:
//   - Evaluates vim-style navigation, paging, cursor control, help, and exit key chords against incoming TermUI event IDs.
//
// Data Flow:
//
//	TermUI Keyboard Event -> keys.go / pkg/keys -> Action Identification -> UI Event Dispatch.
package main

import "github.com/edsilegx/ctop/pkg/keys"

// IsKeyMatch evaluates whether a given TermUI key ID matches a registered logical action.
var IsKeyMatch = keys.IsKeyMatch
