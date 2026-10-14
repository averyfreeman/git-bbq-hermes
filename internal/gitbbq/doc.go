// Package gitbbq contains Git BBQ's project model and repository operations.
//
// It scaffolds project guidance and policy files, reads and validates their
// configuration, records architecture decisions, and regenerates derived
// architecture projections. The manifest, Git habits, and ADR files are the
// inputs; generated indexes and plans are projections of those sources.
//
// The package also exposes planning and execution seams for policy-controlled
// Git actions, language detection, preferences, and
// migration from the legacy AI Software Architect format. It does not invoke
// a language model or implement application code for a scaffolded project.
package gitbbq
