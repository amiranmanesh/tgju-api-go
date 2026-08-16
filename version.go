package tgju

// Version is the release of this library, following semantic versioning.
//
// It is compiled in rather than read from build info so that the value is
// available to [DefaultUserAgent] at package initialisation, and so that a
// caller vendoring the source still reports something meaningful. The binary in
// cmd/tgju overrides its own copy at link time with the git tag.
const Version = "1.0.0"
