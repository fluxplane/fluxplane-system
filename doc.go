// Package system defines primitive host capability contracts.
//
// The package is intentionally protocol-neutral, policy-neutral, and free of
// direct host IO. It exposes primitive capabilities such as file access,
// networking, processes, environment lookup, and clocks. Higher layers own
// workspaces, ACLs, browser automation, human interaction, auth, endpoints,
// datasources, and operation safety policy.
package system
