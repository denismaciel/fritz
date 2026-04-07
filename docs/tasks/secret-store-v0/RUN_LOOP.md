# Run Loop

Rules:

- red, green, refactor
- one ticket at a time
- keep secrets out of prompt context
- keep secrets out of workspace docs
- never expose secret values through `secret_list`

Validation after each ticket:

```bash
go test ./...
uv run scripts/validate_pi_tool_test_parity.py
```
