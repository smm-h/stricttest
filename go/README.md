# stricttest (Go)

Test-environment hygiene helpers for Go suites: a throwaway HOME, an isolated
git config and identity, transport lockdown, credential stripping, and
cleanup-restoring chdir.

```bash
go get github.com/smm-h/stricttest/go
```

```go
import "github.com/smm-h/stricttest/go/hygiene"
```

The helper surface is not implemented yet; the module is published so consumers
can pin it from the first release.

## License

MIT
