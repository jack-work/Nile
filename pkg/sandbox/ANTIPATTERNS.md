# Antipatterns: sandbox

## Active

### Landlock is not implemented — package is a stub [Severity: High]

The package is named `landlock.go` and the architecture doc describes Landlock restrictions, but no actual sandboxing is applied. The neb receives a minimal `PATH` and env vars, but nothing prevents filesystem or network access. The package name creates a false sense of security.

**Fix**: Integrate `github.com/landlock-lsm/go-landlock` with a two-stage fork+restrict+exec pattern. Or use a helper binary that applies Landlock then execs the neb. Until then, rename the package or add prominent warnings.

## Resolved

*(none yet)*
