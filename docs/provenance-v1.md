# rvwasm provenance predicate v1

This document defines the rvwasm-specific provenance predicate used by `rvsmoke` release evidence bundles.

Predicate type URI:

```text
https://github.com/kitaharata/rvwasm/blob/main/docs/provenance-v1.md
```

The corresponding JSON field in rvwasm attestation payloads is `predicate_type`.

This predicate is rvwasm-specific. It is inspired by in-toto/SLSA provenance concepts, but it is not a SLSA Provenance v1 predicate and should not be interpreted as `https://slsa.dev/provenance/v1`.

## Current payload shape

The v1 predicate is emitted as part of an `rvwasm.attestation.v1` payload and may include:

- `schema_version`: the rvwasm attestation schema version.
- `predicate_type`: this document URL.
- `builder`: the builder component, usually `rvsmoke`.
- `invocation`: release and toolchain metadata.
- `materials`: input or derived materials used for the evidence bundle.
- `subjects`: release artifact records included in the evidence bundle.
- `generated_at`: UTC timestamp for payload generation.
- `attestation_sha256`: deterministic SHA-256 over the attestation payload with this field omitted.

## Compatibility notes

Consumers should treat `predicate_type` as an exact string identifier for this rvwasm-specific predicate. Future incompatible predicate changes should use a new document and URI.
