# Antipatterns: protocol

## Active

### ParseResult errors on nil Result field [Severity: Low]

If a response has neither `Error` nor `Result` (e.g., a minimal shutdown ack like `{}`), `r.Result` is `nil`. `json.Unmarshal(nil, target)` returns "unexpected end of JSON input". Currently masked because `shutdown()` discards the return value of `send()`.

**Fix**: Check for `r.Result == nil` before unmarshaling. Return a zero-value result or a distinct sentinel error.

## Resolved

*(none yet)*
