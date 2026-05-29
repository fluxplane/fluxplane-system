// Package mountfs provides a filesystem wrapper with a virtual root and
// mounted subtrees from another system.FileSystem.
//
// Mounted filesystems are policy-neutral building blocks. They expose a mount
// table and coarse read/write access, but they do not own sessions, current
// directories, operation approvals, or engine workspace policy.
package mountfs
